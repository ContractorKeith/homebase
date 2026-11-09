package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ContractorKeith/homebase/internal/model"
)

type fakeHub struct {
	mu       sync.Mutex
	failures int // 503s to serve first
	runSpeed bool
	auths    []string
	batches  []model.NetBatch
}

func (h *fakeHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.auths = append(h.auths, r.Header.Get("Authorization"))
	if r.URL.Path != "/api/ingest/net" || r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	if h.failures > 0 {
		h.failures--
		http.Error(w, "busy", http.StatusServiceUnavailable)
		return
	}
	var b model.NetBatch
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	h.batches = append(h.batches, b)
	json.NewEncoder(w).Encode(model.IngestReply{RunSpeedtest: h.runSpeed})
	h.runSpeed = false
}

var t0 = time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)

func newTestAgent(url string, now *time.Time) *agent {
	rtt := 10.0
	return &agent{
		hub: url, key: "secret", machine: "m1",
		client: http.DefaultClient,
		probe: func(context.Context) []model.NetSample {
			return []model.NetSample{{Machine: "m1", Target: "1.1.1.1", TS: *now, RTTMs: &rtt}}
		},
		speed: func(context.Context) model.SpeedTest {
			d := 90.0
			return model.SpeedTest{Machine: "m1", TS: *now, DownMbps: &d, Server: "test"}
		},
		sleep: func(context.Context, time.Duration) error { return nil },
		now:   func() time.Time { return *now },
		log:   &bytes.Buffer{},
	}
}

func TestRoundPostsBatch(t *testing.T) {
	hub := &fakeHub{}
	srv := httptest.NewServer(hub)
	defer srv.Close()
	now := t0
	a := newTestAgent(srv.URL, &now)
	if err := a.round(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if len(hub.batches) != 1 || hub.auths[0] != "Bearer secret" {
		t.Fatalf("batches=%d auths=%v", len(hub.batches), hub.auths)
	}
	b := hub.batches[0]
	if b.Machine != "m1" || len(b.Samples) != 1 || len(b.Speed) != 1 || b.Samples[0].RTTMs == nil {
		t.Errorf("batch shape: %+v", b)
	}
	if b.Speed[0].UpMbps != nil {
		t.Errorf("unknown upload must stay null, got %v", *b.Speed[0].UpMbps)
	}
}

func TestHubAsksForSpeedtest(t *testing.T) {
	hub := &fakeHub{runSpeed: true}
	srv := httptest.NewServer(hub)
	defer srv.Close()
	now := t0
	a := newTestAgent(srv.URL, &now)
	if err := a.round(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if len(hub.batches) != 2 || len(hub.batches[0].Speed) != 0 || len(hub.batches[1].Speed) != 1 {
		t.Fatalf("want a latency batch then a speed batch, got %+v", hub.batches)
	}
}

func TestNoSpeedtestIgnoresHubRequest(t *testing.T) {
	hub := &fakeHub{runSpeed: true}
	srv := httptest.NewServer(hub)
	defer srv.Close()
	now := t0
	a := newTestAgent(srv.URL, &now)
	a.noSpeed = true
	if err := a.round(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if len(hub.batches) != 1 || len(hub.batches[0].Speed) != 0 {
		t.Errorf("unexpected speed test: %+v", hub.batches)
	}
}

func TestRetryAfter503(t *testing.T) {
	hub := &fakeHub{failures: 2}
	srv := httptest.NewServer(hub)
	defer srv.Close()
	now := t0
	a := newTestAgent(srv.URL, &now)
	var sleeps []time.Duration
	a.sleep = func(_ context.Context, d time.Duration) error { sleeps = append(sleeps, d); return nil }
	if err := a.round(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if len(hub.auths) != 3 || len(hub.batches) != 1 {
		t.Errorf("attempts=%d batches=%d", len(hub.auths), len(hub.batches))
	}
	if len(sleeps) != 2 || sleeps[1] != 2*sleeps[0] {
		t.Errorf("backoff should double, got %v", sleeps)
	}
}

func TestHubDownKeepsSamplesThenResends(t *testing.T) {
	hub := &fakeHub{failures: 3}
	srv := httptest.NewServer(hub)
	defer srv.Close()
	now := t0
	a := newTestAgent(srv.URL, &now)
	if err := a.round(context.Background(), false); err == nil {
		t.Fatal("want error while hub is down")
	}
	if len(a.pending.Samples) != 1 {
		t.Fatalf("held %d samples", len(a.pending.Samples))
	}
	now = now.Add(30 * time.Second)
	if err := a.round(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if len(hub.batches) != 1 || len(hub.batches[0].Samples) != 2 || len(a.pending.Samples) != 0 {
		t.Errorf("want both samples in one batch, got %+v", hub.batches)
	}
}

func TestPendingBoundedTo24h(t *testing.T) {
	now := t0
	a := newTestAgent("http://127.0.0.1:1", &now)
	a.client = &http.Client{Timeout: time.Second}
	a.round(context.Background(), false) // fails, holds one sample
	now = now.Add(25 * time.Hour)
	a.round(context.Background(), false)
	if len(a.pending.Samples) != 1 || !a.pending.Samples[0].TS.Equal(now) {
		t.Errorf("old sample should be dropped: %+v", a.pending.Samples)
	}
}

func TestAgentFlags(t *testing.T) {
	dir := t.TempDir()
	key := filepath.Join(dir, "key")
	os.WriteFile(key, []byte("k\n"), 0o600)
	cases := []struct {
		name string
		args []string
		code int
		msg  string
	}{
		{"no hub", []string{"--key-file", key, "--once"}, 2, "--hub"},
		{"no key", []string{"--hub", "http://x"}, 2, "--key-file"},
		{"bad flag", []string{"--nope"}, 2, ""},
		{"missing key file", []string{"--hub", "http://x", "--key-file", filepath.Join(dir, "none")}, 1, "cannot read"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var out, errb bytes.Buffer
			if got := runAgent(c.args, &out, &errb); got != c.code {
				t.Errorf("exit %d, want %d (%s)", got, c.code, errb.String())
			}
			if !strings.Contains(errb.String(), c.msg) {
				t.Errorf("stderr %q lacks %q", errb.String(), c.msg)
			}
		})
	}
}

func TestAgentDropsBatchTheHubRejects(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()
	now := t0
	a := newTestAgent(srv.URL, &now)
	if err := a.round(context.Background(), false); err == nil {
		t.Fatal("want error on 400")
	}
	if calls != 1 || len(a.pending.Samples) != 0 {
		t.Fatalf("calls %d, pending %d: a 400 must not retry and must drop the batch", calls, len(a.pending.Samples))
	}
}

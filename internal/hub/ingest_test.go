package hub

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ContractorKeith/homebase/internal/model"
	"github.com/ContractorKeith/homebase/internal/store"
)

// ingestServer is a hub over a scratch database.
func ingestServer(t *testing.T) (*httptest.Server, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "hub.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	s := New(Config{Key: key, MM: fakeMM(t, true), Store: st, Logf: t.Logf})
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return ts, st
}

func post(t *testing.T, url string, auth bool, body []byte) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	if auth {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

func jsonOf(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestIngestUsageIdempotentAndCleaned(t *testing.T) {
	ts, st := ingestServer(t)
	h := time.Now().UTC().Truncate(time.Hour).Add(-3 * time.Hour)
	batch := model.UsageBatch{Machine: "m1", Rows: []model.Usage{
		{Machine: "spoofed", Agent: "claude", Model: "opus", Hour: h, Input: 10, Output: -5, Sessions: 1},
		{Agent: "bogus", Model: "x", Hour: h, Input: 99},                               // unknown agent
		{Agent: "codex", Model: "x", Hour: time.Now().Add(3 * time.Hour), Input: 1},    // future
		{Agent: "grok", Model: "x", Hour: time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC)}, // too old
	}}
	for i := 0; i < 2; i++ {
		if r := post(t, ts.URL+"/api/ingest/usage", true, jsonOf(t, batch)); r.StatusCode != 200 {
			t.Fatalf("post %d: %d", i, r.StatusCode)
		}
	}
	got, err := st.UsageBetween(context.Background(), h.Add(-24*time.Hour), time.Now().Add(24*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Machine != "m1" || got[0].Input != 10 || got[0].Output != 0 {
		t.Fatalf("stored %+v", got)
	}
}

func TestIngestRejects(t *testing.T) {
	ts, _ := ingestServer(t)
	url := ts.URL + "/api/ingest/usage"
	if r := post(t, url, false, []byte(`{}`)); r.StatusCode != 401 {
		t.Errorf("no key: %d", r.StatusCode)
	}
	if r := post(t, url, true, []byte(`{not json`)); r.StatusCode != 400 {
		t.Errorf("bad body: %d", r.StatusCode)
	}
	if r := post(t, url, true, []byte(`{"machine":"","rows":[]}`)); r.StatusCode != 400 {
		t.Errorf("empty machine: %d", r.StatusCode)
	}
	big := `{"machine":"m","rows":[{"model":"` + strings.Repeat("a", 9<<20) + `"}]}`
	if r := post(t, url, true, []byte(big)); r.StatusCode != 413 {
		t.Errorf("oversized: %d", r.StatusCode)
	}
	if r := post(t, ts.URL+"/api/ingest/net", true, []byte(`{"samples":[]}`)); r.StatusCode != 400 {
		t.Errorf("net empty machine: %d", r.StatusCode)
	}
}

func TestIngestNetNullSpeedAndSpeedtestFlag(t *testing.T) {
	ts, st := ingestServer(t)
	now := time.Now().UTC().Truncate(time.Second)
	rtt := 12.5
	body := []byte(`{"machine":"m1","samples":[{"target":"1.1.1.1","ts":"` + now.Format(time.RFC3339) +
		`","rttMs":12.5,"jitterMs":null,"lossPct":0}],"speed":[{"ts":"` + now.Format(time.RFC3339) +
		`","downMbps":null,"upMbps":null,"server":"s"}]}`)
	reply := func() model.IngestReply {
		var out model.IngestReply
		r := post(t, ts.URL+"/api/ingest/net", true, body)
		if r.StatusCode != 200 {
			t.Fatalf("net: %d", r.StatusCode)
		}
		json.NewDecoder(r.Body).Decode(&out)
		return out
	}
	if reply().RunSpeedtest {
		t.Fatal("flag set before any request")
	}
	if r := post(t, ts.URL+"/api/speedtest/m1", true, nil); r.StatusCode != 202 {
		t.Fatalf("speedtest: %d", r.StatusCode)
	}
	if r := post(t, ts.URL+"/api/speedtest/m1", false, nil); r.StatusCode != 401 {
		t.Fatalf("speedtest without key: %d", r.StatusCode)
	}
	if !reply().RunSpeedtest {
		t.Fatal("flag missing after request")
	}
	if reply().RunSpeedtest {
		t.Fatal("flag did not clear")
	}
	n, _ := st.NetBetween(context.Background(), now.Add(-time.Hour), now.Add(time.Hour))
	sp, _ := st.SpeedBetween(context.Background(), now.Add(-time.Hour), now.Add(time.Hour))
	if len(n) != 1 || n[0].Machine != "m1" || *n[0].RTTMs != rtt || n[0].JitterMs != nil {
		t.Fatalf("samples %+v", n)
	}
	if len(sp) != 1 || sp[0].DownMbps != nil {
		t.Fatalf("speed %+v", sp)
	}
}

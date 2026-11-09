package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/ContractorKeith/homebase/internal/model"
)

// usageHub records usage batches; status, when set, is served instead of accepting.
type usageHub struct {
	mu      sync.Mutex
	status  int
	batches []model.UsageBatch
}

func (h *usageHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if r.URL.Path != "/api/ingest/usage" {
		http.NotFound(w, r)
		return
	}
	if h.status != 0 {
		http.Error(w, "no", h.status)
		return
	}
	var b model.UsageBatch
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	h.batches = append(h.batches, b)
	json.NewEncoder(w).Encode(model.IngestReply{})
}

func (h *usageHub) rows() (n int) {
	for _, b := range h.batches {
		n += len(b.Rows)
	}
	return n
}

func usageAgent(url string, now *time.Time, rows *[]model.Usage) *agent {
	a := newTestAgent(url, now)
	a.scan = func(context.Context) ([]model.Usage, error) { return *rows, nil }
	return a
}

func urow(hoursAgo int, input int64) model.Usage {
	return model.Usage{Machine: "m1", Agent: "claude", Model: "opus",
		Hour: t0.Add(-time.Duration(hoursAgo) * time.Hour), Input: input}
}

func TestUsageRoundSendsAllThenOnlyRecentOrChanged(t *testing.T) {
	hub := &usageHub{}
	srv := httptest.NewServer(hub)
	defer srv.Close()
	now := t0
	rows := []model.Usage{urow(1, 5), urow(100, 7), urow(200, 9)}
	a := usageAgent(srv.URL, &now, &rows)

	if err := a.usageRound(context.Background()); err != nil || hub.rows() != 3 {
		t.Fatalf("first round: err=%v rows=%d", err, hub.rows())
	}
	hub.batches = nil
	if err := a.usageRound(context.Background()); err != nil || hub.rows() != 1 {
		t.Fatalf("second round (only the last 48 h): err=%v rows=%d", err, hub.rows())
	}
	hub.batches = nil
	rows[2].Input = 10 // an old hour changes
	if err := a.usageRound(context.Background()); err != nil || hub.rows() != 2 {
		t.Fatalf("changed old row: err=%v rows=%d", err, hub.rows())
	}
}

func TestUsageRoundChunks(t *testing.T) {
	hub := &usageHub{}
	srv := httptest.NewServer(hub)
	defer srv.Close()
	now := t0
	var rows []model.Usage
	for i := 0; i < 4500; i++ {
		rows = append(rows, urow(i, 1))
	}
	a := usageAgent(srv.URL, &now, &rows)
	if err := a.usageRound(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(hub.batches) != 3 || len(hub.batches[0].Rows) != 2000 || len(hub.batches[2].Rows) != 500 {
		t.Fatalf("%d batches", len(hub.batches))
	}
}

func TestUsageRoundRetriesTransientThenResendsAll(t *testing.T) {
	hub := &usageHub{status: 503}
	srv := httptest.NewServer(hub)
	defer srv.Close()
	now := t0
	rows := []model.Usage{urow(100, 7)}
	a := usageAgent(srv.URL, &now, &rows)
	if err := a.usageRound(context.Background()); err == nil {
		t.Fatal("expected an error while the hub is down")
	}
	hub.status = 0
	if err := a.usageRound(context.Background()); err != nil || hub.rows() != 1 {
		t.Fatalf("after recovery: err=%v rows=%d (the first round must resend every row)", err, hub.rows())
	}
}

func TestUsageRoundDropsPermanentRefusal(t *testing.T) {
	hub := &usageHub{status: 400}
	srv := httptest.NewServer(hub)
	defer srv.Close()
	now := t0
	rows := []model.Usage{urow(1, 7)}
	a := usageAgent(srv.URL, &now, &rows)
	if err := a.usageRound(context.Background()); err == nil {
		t.Fatal("expected the refusal to be reported")
	}
	if !a.usageFirst || a.usageSent[usageKey(rows[0])] == 0 {
		t.Fatal("a permanently refused chunk must not block later rounds")
	}
}

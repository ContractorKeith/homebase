package hub

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ContractorKeith/homebase/internal/model"
	"github.com/ContractorKeith/homebase/internal/store"
)

var testLoc = func() *time.Location {
	l, err := time.LoadLocation("America/New_York")
	if err != nil {
		return time.FixedZone("EST", -5*3600)
	}
	return l
}()

// fixedNow is Sunday 2026-11-08 15:30 New York, a week after the clocks went back.
var fixedNow = time.Date(2026, 11, 8, 15, 30, 0, 0, testLoc)

// seededServer is a hub over a scratch store holding fixed usage, GitHub and network rows.
func seededServer(t *testing.T) *httptest.Server {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "hub.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ctx := context.Background()
	var usage []model.Usage
	agents := []string{model.AgentClaude, model.AgentCodex, model.AgentGrok}
	for i := 0; i < 330; i++ {
		h := fixedNow.UTC().Truncate(time.Hour).Add(-time.Duration(i) * 31 * time.Hour)
		usage = append(usage, model.Usage{Machine: fmt.Sprintf("m%d", i%2+1), Agent: agents[i%3],
			Model: fmt.Sprintf("model-%d", i%4), Hour: h, Input: int64(100 + i), Output: int64(50 + i%7),
			CacheRead: int64(1000 + i*3), CacheWrite: int64(i % 5 * 10), Reasoning: int64(i % 3 * 4), Sessions: 1})
	}
	for i := 0; i < 6; i++ { // activity today, local hours 00:00 to 15:00
		usage = append(usage, model.Usage{Machine: "m1", Agent: model.AgentClaude, Model: "model-0",
			Hour: time.Date(2026, 11, 8, i*3, 0, 0, 0, testLoc).UTC(), Input: 10, Output: 5, CacheRead: 100, CacheWrite: 2, Reasoning: 1})
	}
	if err := st.UpsertUsage(ctx, usage); err != nil {
		t.Fatal(err)
	}
	var days []model.GitHubDay
	for i := 0; i < 400; i++ {
		d := fixedNow.AddDate(0, 0, -i).Format("2006-01-02")
		for r := 0; r <= i%3; r++ {
			days = append(days, model.GitHubDay{Repo: fmt.Sprintf("keith/repo-%d", (i+r)%12), Day: d, Commits: int64(1 + i%4 + r),
				Additions: int64(10 * (i%5 + 1)), Deletions: int64(i % 6), PRsOpened: int64(i % 2), PRsMerged: int64(r % 2),
				Issues: int64(i % 3 / 2), Reviews: int64(i % 4 / 3)})
		}
	}
	if err := st.ReplaceGitHubDays(ctx, days); err != nil {
		t.Fatal(err)
	}
	rtt, jit, loss := 12.5, 1.5, 0.0
	var samples []model.NetSample
	for i := 0; i < 600; i++ {
		n := model.NetSample{Machine: "m1", Target: "1.1.1.1", TS: fixedNow.Add(-time.Duration(i) * 5 * time.Minute), RTTMs: &rtt, JitterMs: &jit, LossPct: &loss}
		if i%7 == 0 {
			n.RTTMs = nil // unknown stays null
		}
		samples = append(samples, n)
	}
	down, up := 480.0, 22.0
	speed := []model.SpeedTest{{Machine: "m1", TS: fixedNow.Add(-2 * time.Hour), DownMbps: &down, UpMbps: &up, Server: "cf"},
		{Machine: "m1", TS: fixedNow.Add(-8 * time.Hour), DownMbps: &up, UpMbps: &down, Server: "cf"}}
	if err := st.InsertNet(ctx, samples, speed); err != nil {
		t.Fatal(err)
	}
	s := New(Config{Key: key, MM: fakeMM(t, false), Store: st, Logf: t.Logf})
	s.Now = func() time.Time { return fixedNow }
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return ts
}

// checkGolden compares a response body with testdata/golden/<name>; UPDATE_GOLDEN=1 rewrites it.
func checkGolden(t *testing.T, name string, body []byte) {
	t.Helper()
	var buf bytes.Buffer
	if err := json.Indent(&buf, body, "", "  "); err != nil {
		t.Fatalf("%s: %v\n%s", name, err, body)
	}
	buf.WriteByte('\n')
	path := filepath.Join("..", "..", "testdata", "golden", name)
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(want, buf.Bytes()) {
		t.Errorf("%s differs from golden (UPDATE_GOLDEN=1 to rewrite)", name)
	}
}

func fetch(t *testing.T, ts *httptest.Server, path string) (int, []byte) {
	t.Helper()
	r := get(t, ts.URL+path, bearer)
	b, _ := io.ReadAll(r.Body)
	return r.StatusCode, b
}

func TestRangeTokensGolden(t *testing.T) {
	ts := seededServer(t)
	for _, rng := range []string{"today", "7d", "30d", "90d", "ttm"} {
		code, b := fetch(t, ts, "/api/tokens?range="+rng+"&tz=America/New_York")
		if code != 200 {
			t.Fatalf("%s = %d %s", rng, code, b)
		}
		checkGolden(t, "api-tokens-"+rng+".json", b)
	}
}

func TestRangeGitHubGolden(t *testing.T) {
	ts := seededServer(t)
	for _, rng := range []string{"today", "7d", "30d", "90d", "ttm"} {
		code, b := fetch(t, ts, "/api/github?range="+rng+"&tz=America/New_York")
		if code != 200 {
			t.Fatalf("%s = %d %s", rng, code, b)
		}
		checkGolden(t, "api-github-"+rng+".json", b)
	}
}

func TestRangeBadParams(t *testing.T) {
	ts := seededServer(t)
	for _, p := range []string{"/api/tokens?range=1y", "/api/tokens?range=7d&tz=Mars/Base", "/api/tokens?range=7d&by=color",
		"/api/github?range=", "/api/github?range=7d&tz=nope", "/api/net?range=1y"} {
		if code, _ := fetch(t, ts, p); code != 400 {
			t.Errorf("%s = %d, want 400", p, code)
		}
	}
}

func TestTokensTotalsAndSlots(t *testing.T) {
	ts := seededServer(t)
	_, b := fetch(t, ts, "/api/tokens?range=today&tz=America/New_York&by=agent")
	var r struct {
		Totals     tokenCounts
		PrevTotals tokenCounts
		Series     []tokenSeries
	}
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	c := r.Totals
	if c.Total != c.Input+c.Output+c.CacheRead+c.CacheWrite || c.Total == 0 {
		t.Errorf("total %d is not input+output+cacheRead+cacheWrite of %+v", c.Total, c)
	}
	slot := map[string]int{}
	for _, s := range r.Series {
		slot[s.Key] = s.Slot
	}
	_, b = fetch(t, ts, "/api/tokens?range=ttm&tz=America/New_York&by=agent")
	json.Unmarshal(b, &r)
	for _, s := range r.Series {
		if old, ok := slot[s.Key]; ok && old != s.Slot {
			t.Errorf("%s moved from slot %d to %d between ranges", s.Key, old, s.Slot)
		}
	}
}

func TestNet(t *testing.T) {
	ts := seededServer(t)
	code, b := fetch(t, ts, "/api/net?range=24h")
	if code != 200 {
		t.Fatalf("net = %d %s", code, b)
	}
	var r struct {
		Latest []model.NetSample
		Speed  []model.SpeedTest
		Series []netSeries
	}
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	if len(r.Latest) != 1 || len(r.Speed) != 1 || *r.Speed[0].DownMbps != 480 {
		t.Errorf("latest %d speed %+v", len(r.Latest), r.Speed)
	}
	if len(r.Series) != 1 || len(r.Series[0].Points) == 0 || len(r.Series[0].Points) > maxNetPoints {
		t.Errorf("series %+v", len(r.Series))
	}
	_, b = fetch(t, ts, "/api/net?range=7d&machine=nobody")
	if string(bytes.TrimSpace(b)) != `{"latest":[],"series":[],"speed":[]}` {
		t.Errorf("unknown machine = %s", b)
	}
}

package hub

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ContractorKeith/homebase/internal/mm"
)

const key = "secret-key"

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("../../testdata/mm/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// fakeMM serves the recorded machine-monitor JSON; with down set it fails every request.
func fakeMM(t *testing.T, down bool) *mm.Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/devices", func(w http.ResponseWriter, r *http.Request) { w.Write(fixture(t, "devices.json")) })
	mux.HandleFunc("/api/history/{id}", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("id") != "studio-1" {
			http.NotFound(w, r)
			return
		}
		w.Write(fixture(t, "history.json"))
	})
	mux.HandleFunc("/api/stream", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write(fixture(t, "stream.sse"))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	url := srv.URL
	if down {
		url = "http://127.0.0.1:1"
	}
	c := mm.New(url, "")
	c.RetryMin, c.RetryMax = 10*time.Millisecond, 20*time.Millisecond
	return c
}

func newServer(t *testing.T, down bool) *httptest.Server {
	t.Helper()
	s := New(Config{Key: key, MM: fakeMM(t, down), Logf: t.Logf})
	ctx, cancel := context.WithCancel(context.Background())
	go s.Run(ctx)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(func() { ts.Close(); cancel() })
	return ts
}

func get(t *testing.T, url string, hdr map[string]string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest("GET", url, nil)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	c := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

var bearer = map[string]string{"Authorization": "Bearer " + key}

func TestAuth(t *testing.T) {
	ts := newServer(t, false)
	for _, p := range []string{"/", "/api/machines", "/api/stream", "/api/machines/studio-1/history", "/js/main.js"} {
		if c := get(t, ts.URL+p, nil).StatusCode; c != 401 {
			t.Errorf("%s without key = %d", p, c)
		}
		if c := get(t, ts.URL+p, map[string]string{"Authorization": "Bearer nope"}).StatusCode; c != 401 {
			t.Errorf("%s with wrong key = %d", p, c)
		}
	}
	if c := get(t, ts.URL+"/?key=nope", nil).StatusCode; c != 401 {
		t.Errorf("wrong ?key = %d", c)
	}
	// Visiting /?key= sets the cookie, which then authorises later requests.
	r := get(t, ts.URL+"/?key="+key, nil)
	if r.StatusCode != 303 || len(r.Cookies()) != 1 {
		t.Fatalf("?key= gave %d with %d cookies", r.StatusCode, len(r.Cookies()))
	}
	if c := get(t, ts.URL+"/api/machines", map[string]string{"Cookie": r.Cookies()[0].String()}).StatusCode; c != 200 {
		t.Errorf("cookie request = %d", c)
	}
}

func TestMachines(t *testing.T) {
	ts := newServer(t, false)
	r := get(t, ts.URL+"/api/machines", bearer)
	var ms []map[string]any
	if err := json.NewDecoder(r.Body).Decode(&ms); err != nil || len(ms) != 3 {
		t.Fatalf("machines: %v, %d", err, len(ms))
	}
	studio, air := ms[0], ms[1]
	if studio["name"] != "Mac Studio" || studio["uptime"].(float64) != 864000 {
		t.Errorf("studio = %v", studio)
	}
	if air["uptime"] != nil || len(air["gpus"].([]any)) != 0 || air["cpu"].(map[string]any)["usage"] != nil {
		t.Errorf("unknown metrics must be null and lists empty: %v", air)
	}
	if _, ok := studio["netq"]; !ok {
		t.Error("netq slot missing")
	}
	if cores := studio["cpu"].(map[string]any)["cores"].([]any); cores[3] != nil {
		t.Error("null core must stay null")
	}
}

func TestMachinesMMDown(t *testing.T) {
	ts := newServer(t, true)
	if c := get(t, ts.URL+"/api/machines", bearer).StatusCode; c != 502 {
		t.Errorf("mm down = %d, want 502", c)
	}
}

func TestHistory(t *testing.T) {
	ts := newServer(t, false)
	r := get(t, ts.URL+"/api/machines/studio-1/history?range=24h", bearer)
	var h mm.History
	if err := json.NewDecoder(r.Body).Decode(&h); err != nil || r.StatusCode != 200 || len(h.Points) != 2 {
		t.Fatalf("history: %v %d %+v", err, r.StatusCode, h)
	}
	if c := get(t, ts.URL+"/api/machines/studio-1/history?range=90d", bearer).StatusCode; c != 400 {
		t.Errorf("bad range = %d", c)
	}
	if c := get(t, ts.URL+"/api/machines/ghost/history?range=1h", bearer).StatusCode; c != 404 {
		t.Errorf("unknown machine = %d", c)
	}
}

func TestStream(t *testing.T) {
	ts := newServer(t, false)
	r := get(t, ts.URL+"/api/stream", bearer)
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content type %q", ct)
	}
	// Read until a "machine" update with the new CPU value arrives.
	deadline := time.AfterFunc(5*time.Second, func() { r.Body.Close() })
	defer deadline.Stop()
	buf := make([]byte, 0, 1<<16)
	tmp := make([]byte, 4096)
	for !strings.Contains(string(buf), `"type":"machine"`) {
		n, err := r.Body.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			t.Fatalf("stream ended: %v\n%s", err, buf)
		}
	}
	if !strings.Contains(string(buf), `"type":"snapshot"`) || !strings.Contains(string(buf), `"usage":55`) {
		t.Errorf("stream = %s", buf)
	}
}

func TestWebEmbedded(t *testing.T) {
	ts := newServer(t, false)
	r := get(t, ts.URL+"/", bearer)
	b := make([]byte, 1<<16)
	n, _ := r.Body.Read(b)
	if r.StatusCode != 200 || !strings.Contains(string(b[:n]), "uPlot") {
		t.Errorf("index = %d", r.StatusCode)
	}
	if c := get(t, ts.URL+"/vendor/uplot/uPlot.iife.min.js", bearer).StatusCode; c != 200 {
		t.Errorf("uPlot js = %d", c)
	}
}

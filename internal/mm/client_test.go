package mm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

const testKey = "k3y"

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("../../testdata/mm/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// fakeHub serves the recorded JSON and checks the Bearer key like machine-monitor does.
func fakeHub(t *testing.T, stream http.HandlerFunc) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/devices", func(w http.ResponseWriter, r *http.Request) {
		w.Write(fixture(t, "devices.json"))
	})
	mux.HandleFunc("/api/history/{id}", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("id") != "studio-1" || r.URL.Query().Get("range") != "24h" {
			http.NotFound(w, r)
			return
		}
		w.Write(fixture(t, "history.json"))
	})
	if stream != nil {
		mux.HandleFunc("/api/stream", stream)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+testKey {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestDevices(t *testing.T) {
	srv := fakeHub(t, nil)
	devs, err := New(srv.URL, testKey).Devices(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(devs) != 3 {
		t.Fatalf("got %d devices", len(devs))
	}
	studio, air, fresh := devs[0], devs[1], devs[2]
	if !studio.Online || *studio.Latest.CPU.Usage != 23.5 || len(studio.Latest.CPU.Cores) != 8 {
		t.Errorf("studio = %+v", studio.Latest.CPU)
	}
	if studio.Latest.CPU.Cores[3] != nil || *studio.Latest.CPU.Cores[7] != 100 {
		t.Error("a null core must stay nil")
	}
	if g := studio.Latest.GPUs[0]; *g.MemTotal != 68719476736 || *g.Power != 9.5 {
		t.Errorf("gpu = %+v", g)
	}
	if studio.Latest.Temps[1].Value != nil {
		t.Error("null temp must stay nil")
	}
	if *studio.Latest.Net.RX != 125000 {
		t.Error("net rx")
	}
	if len(air.Latest.GPUs) != 0 || air.Latest.CPU.Usage != nil || air.Latest.Net != nil || air.Latest.Uptime != nil {
		t.Errorf("no-GPU device with nil metrics = %+v", air.Latest)
	}
	if fresh.Latest != nil {
		t.Error("a device that never reported has nil Latest")
	}
}

func TestHistory(t *testing.T) {
	srv := fakeHub(t, nil)
	c := New(srv.URL, testKey)
	h, err := c.History(context.Background(), "studio-1", "24h")
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Points) != 2 || h.Points[0].GPU != nil || *h.Points[1].GPU != 10 || h.Points[1].CPUTemp != nil {
		t.Errorf("points = %+v", h.Points)
	}
	_, err = c.History(context.Background(), "nope", "24h")
	if se, ok := err.(*StatusError); !ok || se.Code != 404 {
		t.Errorf("err = %v, want 404 StatusError", err)
	}
}

func TestUnauthorized(t *testing.T) {
	srv := fakeHub(t, func(w http.ResponseWriter, r *http.Request) {})
	c := New(srv.URL, "wrong")
	if _, err := c.Devices(context.Background()); err != ErrUnauthorized {
		t.Errorf("Devices err = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var last Event
	for e := range c.Stream(ctx) { // the channel must close: a wrong key is not retried
		last = e
	}
	if last.Type != EventDisconnected || last.Err != ErrUnauthorized {
		t.Errorf("last event = %+v", last)
	}
}

// The first connection ends after its events; the second stays open. The client must
// report the drop, back off, reconnect and deliver a fresh snapshot.
func TestStreamReconnects(t *testing.T) {
	var conns atomic.Int32
	srv := fakeHub(t, func(w http.ResponseWriter, r *http.Request) {
		n := conns.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write(fixture(t, "stream.sse"))
		w.(http.Flusher).Flush()
		if n > 1 {
			<-r.Context().Done()
		}
	})
	c := New(srv.URL, testKey)
	c.RetryMin, c.RetryMax = 10*time.Millisecond, 20*time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var types []string
	snapshots := 0
	for e := range c.Stream(ctx) {
		types = append(types, e.Type)
		switch e.Type {
		case EventSnapshot:
			snapshots++
			if len(e.Devices) != 1 || *e.Devices[0].Latest.CPU.Usage != 10 {
				t.Errorf("snapshot = %+v", e.Devices)
			}
		case EventReport:
			if e.ID != "studio-1" || *e.Device.Latest.CPU.Usage != 55 || *e.Device.Latest.Net.TX != 2 {
				t.Errorf("report = %+v", e.Device)
			}
		case EventDisconnected:
			if e.Retry <= 0 {
				t.Errorf("disconnect has no retry: %+v", e)
			}
		case EventStatus:
			if e.Online["air-1"] || !e.Online["studio-1"] {
				t.Errorf("status = %v", e.Online)
			}
		}
		if snapshots == 2 {
			break
		}
	}
	want := []string{"connected", "snapshot", "report", "status", "disconnected", "connected", "snapshot"}
	if len(types) != len(want) {
		t.Fatalf("events = %v, want %v", types, want)
	}
	for i := range want {
		if types[i] != want[i] {
			t.Fatalf("events = %v, want %v", types, want)
		}
	}
}

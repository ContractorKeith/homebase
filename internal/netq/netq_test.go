package netq

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("../../testdata/netq/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func eq(a, b *float64) bool {
	if a == nil || b == nil {
		return a == b
	}
	d := *a - *b
	return d > -1e-9 && d < 1e-9
}

func f(v float64) *float64 { return &v }

func TestParsePing(t *testing.T) {
	cases := []struct {
		file             string
		rtt, jitter, los *float64
	}{
		{"linux-ok.txt", f(10.238), f(0.887), f(0)},
		{"linux-loss.txt", f(14.8), f(10.113), f(30)},
		{"linux-100loss.txt", nil, nil, f(100)},
		{"linux-errors.txt", nil, nil, f(100)},
		{"linux-unreachable.txt", nil, nil, nil},
		{"darwin-ok.txt", f(11.041), f(1.790), f(0)},
		{"darwin-loss.txt", f(14.8), f(10.113), f(30)},
		{"darwin-100loss.txt", nil, nil, f(100)},
		{"darwin-unreachable.txt", nil, nil, nil},
	}
	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			r := ParsePing(fixture(t, c.file))
			if !eq(r.RTTMs, c.rtt) || !eq(r.JitterMs, c.jitter) || !eq(r.LossPct, c.los) {
				t.Errorf("got rtt=%v jitter=%v loss=%v", r.RTTMs, r.JitterMs, r.LossPct)
			}
		})
	}
}

func TestPingArgs(t *testing.T) {
	if got := strings.Join(pingArgs("linux", "h"), " "); got != "-n -c 10 -i 0.2 -W 1 h" {
		t.Errorf("linux: %s", got)
	}
	if got := strings.Join(pingArgs("darwin", "h"), " "); got != "-n -c 10 -i 0.2 -W 1000 h" {
		t.Errorf("darwin: %s", got)
	}
}

// fake routes commands to fixture output.
func fake(t *testing.T, route string, pingOut map[string]string) Runner {
	return func(_ context.Context, name string, args ...string) (string, error) {
		switch name {
		case "ip", "route":
			return route, nil
		case "ping":
			if out, ok := pingOut[args[len(args)-1]]; ok {
				return out, nil
			}
		}
		return "", errors.New("exec: not found")
	}
}

func prober(goos string, run Runner) Prober {
	return Prober{Machine: "m1", GOOS: goos, Run: run, Now: func() time.Time { return time.Unix(1000, 0) }}
}

func TestGateway(t *testing.T) {
	ctx := context.Background()
	if g := Gateway(ctx, fake(t, fixture(t, "ip-route-linux.txt"), nil), "linux"); g != "192.168.0.1" {
		t.Errorf("linux gateway %q", g)
	}
	if g := Gateway(ctx, fake(t, fixture(t, "route-darwin.txt"), nil), "darwin"); g != "192.168.1.1" {
		t.Errorf("darwin gateway %q", g)
	}
	if g := Gateway(ctx, fake(t, fixture(t, "route-darwin-v6.txt"), nil), "darwin"); g != "" {
		t.Errorf("v6 gateway should be skipped, got %q", g)
	}
	if g := Gateway(ctx, fake(t, fixture(t, "ip-route-empty.txt"), nil), "linux"); g != "" {
		t.Errorf("empty route gave %q", g)
	}
}

func TestProbe(t *testing.T) {
	run := fake(t, fixture(t, "ip-route-linux.txt"), map[string]string{
		"192.168.0.1": fixture(t, "linux-loss.txt"),
		"1.1.1.1":     fixture(t, "linux-ok.txt"),
	})
	got := prober("linux", run).Probe(context.Background())
	if len(got) != 2 || got[0].Target != "gateway" || got[1].Target != "1.1.1.1" {
		t.Fatalf("samples: %+v", got)
	}
	if !eq(got[0].LossPct, f(30)) || !eq(got[1].RTTMs, f(10.238)) || got[0].Machine != "m1" {
		t.Errorf("values: %+v %+v", got[0], got[1])
	}
}

func TestProbeSkipsUnknownGateway(t *testing.T) {
	run := fake(t, "", map[string]string{"1.1.1.1": fixture(t, "darwin-ok.txt")})
	got := prober("darwin", run).Probe(context.Background())
	if len(got) != 1 || got[0].Target != "1.1.1.1" || !eq(got[0].JitterMs, f(1.79)) {
		t.Errorf("samples: %+v", got)
	}
}

func TestProbeMissingPingIsNil(t *testing.T) {
	got := prober("linux", fake(t, fixture(t, "ip-route-linux.txt"), nil)).Probe(context.Background())
	if len(got) != 2 {
		t.Fatalf("want 2 samples, got %d", len(got))
	}
	for _, s := range got {
		if s.RTTMs != nil || s.JitterMs != nil || s.LossPct != nil {
			t.Errorf("want all nil, got %+v", s)
		}
	}
}

func speedServer(t *testing.T, delay time.Duration) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/__down":
			n := 2_000_000
			w.Header().Set("Content-Length", "2000000")
			w.Write(make([]byte, 1000)) // first byte arrives early
			w.(http.Flusher).Flush()
			select {
			case <-time.After(delay):
			case <-r.Context().Done():
				return
			}
			w.Write(make([]byte, n-1000))
		case "/__up":
			select {
			case <-time.After(delay):
			case <-r.Context().Done():
				return
			}
			b := make([]byte, 32<<10)
			for {
				if _, err := r.Body.Read(b); err != nil {
					break
				}
			}
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestSpeed(t *testing.T) {
	srv := speedServer(t, 20*time.Millisecond)
	st := Speed{BaseURL: srv.URL, Client: srv.Client(), Bytes: 2_000_000}.Run(context.Background(), "m1")
	if st.DownMbps == nil || st.UpMbps == nil || *st.DownMbps <= 0 || *st.UpMbps <= 0 {
		t.Fatalf("want positive speeds, got %+v", st)
	}
	if st.Machine != "m1" || st.Server == "" {
		t.Errorf("meta: %+v", st)
	}
}

func TestSpeedTimeoutIsNil(t *testing.T) {
	srv := speedServer(t, 700*time.Millisecond)
	st := Speed{BaseURL: srv.URL, Client: srv.Client(), Bytes: 2_000_000, Timeout: 100 * time.Millisecond}.Run(context.Background(), "m1")
	if st.DownMbps != nil || st.UpMbps != nil {
		t.Errorf("timed-out legs must be nil, got down=%v up=%v", st.DownMbps, st.UpMbps)
	}
}

func TestSpeedServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(500) }))
	defer srv.Close()
	st := Speed{BaseURL: srv.URL, Client: srv.Client()}.Run(context.Background(), "m1")
	if st.DownMbps != nil || st.UpMbps != nil {
		t.Errorf("500 must give nil, got %+v", st)
	}
}

func TestSpeedCapsBytes(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/__down" {
			got = r.URL.Query().Get("bytes")
		}
		w.WriteHeader(500)
	}))
	defer srv.Close()
	Speed{BaseURL: srv.URL, Client: srv.Client(), Bytes: 999_000_000}.Run(context.Background(), "m")
	if got != "25000000" {
		t.Errorf("bytes=%s, want 25000000", got)
	}
}

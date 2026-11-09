package netq

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/ContractorKeith/homebase/internal/model"
)

const (
	// DefaultSpeedURL is Cloudflare's speed test endpoint.
	DefaultSpeedURL = "https://speed.cloudflare.com"
	maxLegBytes     = 25_000_000 // cap per leg
	legTimeout      = 30 * time.Second
)

// Speed runs download and upload legs against a Cloudflare-compatible server.
type Speed struct {
	BaseURL string // DefaultSpeedURL when empty
	Client  *http.Client
	Bytes   int64         // per-leg size, capped at 25 MB; 0 means the cap
	Timeout time.Duration // per-leg; 0 means 30 s
	Now     func() time.Time
}

// Run measures both legs. A failed or timed-out leg is nil, never 0.
func (s Speed) Run(ctx context.Context, machine string) model.SpeedTest {
	base := s.BaseURL
	if base == "" {
		base = DefaultSpeedURL
	}
	n := s.Bytes
	if n <= 0 || n > maxLegBytes {
		n = maxLegBytes
	}
	st := model.SpeedTest{Machine: machine, TS: s.now().UTC()}
	if u, err := url.Parse(base); err == nil {
		st.Server = u.Host
	}
	st.DownMbps = s.leg(ctx, func(c context.Context) (float64, error) { return s.down(c, base, n) })
	st.UpMbps = s.leg(ctx, func(c context.Context) (float64, error) { return s.up(c, base, n) })
	return st
}

func (s Speed) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s Speed) leg(ctx context.Context, f func(context.Context) (float64, error)) *float64 {
	t := s.Timeout
	if t <= 0 {
		t = legTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, t)
	defer cancel()
	mbps, err := f(ctx)
	if err != nil || mbps <= 0 {
		return nil
	}
	return &mbps
}

func (s Speed) client() *http.Client {
	if s.Client != nil {
		return s.Client
	}
	return http.DefaultClient
}

// down times the body after its first byte arrives, so connection setup and server
// think-time are excluded.
func (s Speed) down(ctx context.Context, base string, n int64) (float64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/__down?bytes="+strconv.FormatInt(n, 10), nil)
	if err != nil {
		return 0, err
	}
	resp, err := s.client().Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, httpError(resp)
	}
	buf := make([]byte, 64<<10)
	first, err := resp.Body.Read(buf)
	for first == 0 && err == nil {
		first, err = resp.Body.Read(buf)
	}
	if first == 0 {
		return 0, err
	}
	start := time.Now()
	rest, err := io.Copy(io.Discard, resp.Body)
	if err != nil {
		return 0, err
	}
	return mbps(rest, time.Since(start)), nil
}

// up posts n zero bytes and times from the first body read to the server's reply.
func (s Speed) up(ctx context.Context, base string, n int64) (float64, error) {
	body := &stampReader{r: io.LimitReader(zeros{}, n)}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/__up", body)
	if err != nil {
		return 0, err
	}
	req.ContentLength = n
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := s.client().Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return 0, httpError(resp)
	}
	if body.start.IsZero() {
		return 0, io.ErrUnexpectedEOF
	}
	return mbps(n, time.Since(body.start)), nil
}

func mbps(bytes int64, d time.Duration) float64 {
	if bytes <= 0 || d <= 0 {
		return 0
	}
	return float64(bytes) * 8 / 1e6 / d.Seconds()
}

type zeros struct{}

func (zeros) Read(p []byte) (int, error) { clear(p); return len(p), nil }

// stampReader records when the transport first reads the body.
type stampReader struct {
	r     io.Reader
	start time.Time
}

func (s *stampReader) Read(p []byte) (int, error) {
	if s.start.IsZero() {
		s.start = time.Now()
	}
	return s.r.Read(p)
}

type statusError int

func (e statusError) Error() string { return "speed server returned " + strconv.Itoa(int(e)) }

func httpError(r *http.Response) error { return statusError(r.StatusCode) }

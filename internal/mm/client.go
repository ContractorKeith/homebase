package mm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const requestTimeout = 15 * time.Second

// ErrUnauthorized means machine-monitor refused the key. Retrying will not help.
var ErrUnauthorized = errors.New("mm: the hub refused the key")

// StatusError is any other non-200 reply.
type StatusError struct{ Code int }

func (e *StatusError) Error() string { return fmt.Sprintf("mm: hub replied %d", e.Code) }

// Client talks to one machine-monitor hub. It is safe for concurrent use.
type Client struct {
	base   string
	key    string
	http   *http.Client // requests, with a timeout
	stream *http.Client // the SSE stream: no overall timeout

	// Reconnect backoff for Stream; New sets 1 s and 30 s, tests shrink them.
	RetryMin, RetryMax time.Duration
}

// New returns a client for the hub at baseURL. key may be empty for a hub that needs none.
func New(baseURL, key string) *Client {
	return &Client{
		base:     strings.TrimRight(baseURL, "/"),
		key:      key,
		http:     &http.Client{Timeout: requestTimeout},
		stream:   &http.Client{},
		RetryMin: time.Second,
		RetryMax: 30 * time.Second,
	}
}

func (c *Client) newRequest(ctx context.Context, path string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return nil, err
	}
	if c.key != "" {
		req.Header.Set("Authorization", "Bearer "+c.key)
	}
	return req, nil
}

func (c *Client) getJSON(ctx context.Context, path string, out any) error {
	req, err := c.newRequest(ctx, path)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := checkStatus(resp); err != nil {
		return err
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 64<<20)).Decode(out)
}

func checkStatus(resp *http.Response) error {
	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return ErrUnauthorized
	}
	return &StatusError{resp.StatusCode}
}

// Devices is GET /api/devices: every machine with its latest report.
func (c *Client) Devices(ctx context.Context) ([]Device, error) {
	var out []Device
	if err := c.getJSON(ctx, "/api/devices?compact=1", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// History is GET /api/history/:id?range=: the machine's history, downsampled by the hub.
// range is one of machine-monitor's names (10m, 1h, 6h, 24h, 7d, 30d).
func (c *Client) History(ctx context.Context, id, rng string) (*History, error) {
	var out History
	if err := c.getJSON(ctx, "/api/history/"+url.PathEscape(id)+"?range="+url.QueryEscape(rng), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

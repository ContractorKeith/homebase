package mm

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"strings"
	"time"
)

// Event types. Connected and Disconnected are made by the client; the rest come from the hub.
const (
	EventConnected    = "connected"
	EventDisconnected = "disconnected" // Err and Retry are set
	EventSnapshot     = "snapshot"     // Devices: every machine; replaces what you had
	EventReport       = "report"       // Device: one machine's new state
	EventDevice       = "device"       // Device: renamed
	EventStatus       = "status"       // Online: id -> online
	EventRemoved      = "removed"      // ID
)

// Event is one message from the stream; only the fields its Type uses are set.
type Event struct {
	Type    string
	Devices []Device
	Device  *Device
	ID      string
	Online  map[string]bool
	Err     error
	Retry   time.Duration
}

type wireEvent struct {
	Type    string          `json:"type"`
	Devices []Device        `json:"devices"`
	Device  *Device         `json:"device"`
	ID      string          `json:"id"`
	Online  map[string]bool `json:"online"`
}

// idleTimeout drops a stream that sent nothing: the hub sends a status event every 3 s.
const idleTimeout = 20 * time.Second

// Stream follows GET /api/stream until ctx ends, reconnecting with backoff. Every
// (re)connection sends EventConnected and then the hub's snapshot. The channel closes when
// ctx ends, or after a Disconnected whose Err is ErrUnauthorized.
func (c *Client) Stream(ctx context.Context) <-chan Event {
	out := make(chan Event, 16)
	send := func(e Event) bool {
		select {
		case out <- e:
			return true
		case <-ctx.Done():
			return false
		}
	}
	go func() {
		defer close(out)
		var delay time.Duration
		for {
			gotSnapshot, err := c.streamOnce(ctx, send)
			if ctx.Err() != nil {
				return
			}
			if err == ErrUnauthorized {
				send(Event{Type: EventDisconnected, Err: err})
				return
			}
			if gotSnapshot {
				delay = 0 // a good connection restarts the backoff
			}
			delay = c.nextDelay(delay)
			if !send(Event{Type: EventDisconnected, Err: err, Retry: delay}) {
				return
			}
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

// nextDelay doubles prev from RetryMin up to RetryMax, with up to 25 % jitter.
func (c *Client) nextDelay(prev time.Duration) time.Duration {
	d := c.RetryMin
	if prev > 0 {
		d = max(min(prev*2, c.RetryMax), c.RetryMin)
	}
	return d + time.Duration(rand.Int64N(int64(d)/4+1))
}

// streamOnce runs one connection until it fails.
func (c *Client) streamOnce(ctx context.Context, send func(Event) bool) (gotSnapshot bool, err error) {
	cctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	req, err := c.newRequest(cctx, "/api/stream?compact=1")
	if err != nil {
		return false, err
	}
	req.Header.Set("Accept", "text/event-stream")
	resp, err := c.stream.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if err := checkStatus(resp); err != nil {
		return false, err
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		return false, fmt.Errorf("mm: stream has Content-Type %q", ct)
	}
	if !send(Event{Type: EventConnected}) {
		return false, ctx.Err()
	}
	idle := time.AfterFunc(idleTimeout, func() { cancel(fmt.Errorf("mm: stream went quiet")) })
	defer idle.Stop()

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 64<<10), 64<<20)
	for sc.Scan() {
		idle.Reset(idleTimeout)
		data, ok := strings.CutPrefix(sc.Text(), "data:")
		if !ok { // comments, retry:, blank lines
			continue
		}
		var w wireEvent
		if json.Unmarshal([]byte(strings.TrimSpace(data)), &w) != nil || w.Type == "" {
			continue // not a shape we know: skip it, keep the stream
		}
		ev := Event{Type: w.Type, Devices: w.Devices, Device: w.Device, ID: w.ID, Online: w.Online}
		switch w.Type {
		case EventSnapshot:
			gotSnapshot = true
		case EventReport, EventDevice, EventStatus, EventRemoved:
		default:
			continue
		}
		if !send(ev) {
			return gotSnapshot, ctx.Err()
		}
	}
	if cause := context.Cause(cctx); cause != nil && ctx.Err() == nil {
		return gotSnapshot, cause
	}
	if err := sc.Err(); err != nil {
		return gotSnapshot, err
	}
	return gotSnapshot, fmt.Errorf("mm: stream ended")
}

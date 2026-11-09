package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ContractorKeith/homebase/internal/model"
)

type speedResult = model.SpeedTest

const (
	maxHold     = 24 * time.Hour // unsent samples older than this are dropped
	postTries   = 3
	firstDelay  = time.Second // backoff doubles: 1 s, 2 s
	maxBodyRead = 1 << 20
)

// agent holds one machine's probe state. Every outside effect is a field so tests can fake it.
type agent struct {
	hub, key, machine string
	client            *http.Client
	probe             func(context.Context) []model.NetSample
	speed             func(context.Context) speedResult
	noSpeed           bool
	interval          time.Duration
	speedEvery        time.Duration
	sleep             func(context.Context, time.Duration) error
	now               func() time.Time
	log               io.Writer
	pending           model.NetBatch // not yet accepted by the hub

	scan       func(context.Context) ([]model.Usage, error) // local token usage; nil turns usage off
	usageEvery time.Duration
	usageSent  map[string]uint64 // row key -> hash of the last row the hub accepted
	usageFirst bool              // the all-rows round has completed
}

// round probes, optionally speed-tests, and posts everything pending. If the hub asks for
// a speed test in its reply, it runs one and posts that too.
func (a *agent) round(ctx context.Context, withSpeed bool) error {
	a.pending.Samples = append(a.pending.Samples, a.probe(ctx)...)
	if withSpeed {
		a.pending.Speed = append(a.pending.Speed, a.speed(ctx))
	}
	a.prune()
	reply, err := a.post(ctx)
	if err != nil {
		return err
	}
	if reply.RunSpeedtest && !a.noSpeed && !withSpeed {
		a.pending.Speed = append(a.pending.Speed, a.speed(ctx))
		_, err = a.post(ctx)
	}
	return err
}

// prune drops held rows older than maxHold.
func (a *agent) prune() {
	cut := a.now().Add(-maxHold)
	s := a.pending.Samples[:0]
	for _, x := range a.pending.Samples {
		if x.TS.After(cut) {
			s = append(s, x)
		}
	}
	a.pending.Samples = s
	t := a.pending.Speed[:0]
	for _, x := range a.pending.Speed {
		if x.TS.After(cut) {
			t = append(t, x)
		}
	}
	a.pending.Speed = t
}

// post sends the pending net batch and clears it on success or a permanent refusal.
func (a *agent) post(ctx context.Context) (model.IngestReply, error) {
	a.pending.Machine = a.machine
	body, err := json.Marshal(a.pending)
	if err != nil {
		return model.IngestReply{}, err
	}
	reply, err := a.send(ctx, "/api/ingest/net", body)
	if err == nil || isPermanent(err) {
		// A permanent refusal means the hub rejected this batch's content; resending it
		// would block every later round, so drop it.
		a.pending = model.NetBatch{}
	}
	return reply, err
}

func isPermanent(err error) bool {
	var he hubStatus
	return errors.As(err, &he) && he.permanent()
}

// send posts body to path, retrying transient failures with backoff.
func (a *agent) send(ctx context.Context, path string, body []byte) (model.IngestReply, error) {
	delay := firstDelay
	for try := 1; ; try++ {
		reply, err := a.postOnce(ctx, path, body)
		if err == nil || isPermanent(err) || try == postTries || ctx.Err() != nil {
			return reply, err
		}
		if a.sleep(ctx, delay) != nil {
			return reply, err
		}
		delay *= 2
	}
}

func (a *agent) postOnce(ctx context.Context, path string, body []byte) (model.IngestReply, error) {
	var reply model.IngestReply
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.hub+path, bytes.NewReader(body))
	if err != nil {
		return reply, err
	}
	req.Header.Set("Authorization", "Bearer "+a.key)
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		return reply, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, maxBodyRead))
	if resp.StatusCode/100 != 2 {
		return reply, hubStatus(resp.StatusCode)
	}
	if len(bytes.TrimSpace(b)) > 0 {
		if err := json.Unmarshal(b, &reply); err != nil {
			return reply, fmt.Errorf("bad hub reply: %w", err)
		}
	}
	return reply, nil
}

// hubStatus is a non-2xx reply from the hub.
type hubStatus int

func (h hubStatus) Error() string {
	return fmt.Sprintf("hub returned %d %s", int(h), http.StatusText(int(h)))
}

// permanent reports a 4xx that resending cannot fix. 401/403 (wrong key), 408 and 429 are
// held and retried, since fixing the key or waiting makes them succeed.
func (h hubStatus) permanent() bool {
	switch int(h) {
	case 401, 403, 408, 429:
		return false
	}
	return h >= 400 && h < 500
}

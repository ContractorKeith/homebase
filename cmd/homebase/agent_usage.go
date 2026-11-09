package main

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/ContractorKeith/homebase/internal/model"
	"github.com/ContractorKeith/homebase/internal/tokens"
)

const (
	usageChunk  = 2000
	usageWindow = 48 * time.Hour // rows this recent are always resent
)

// defaultUsageState is $XDG_STATE_HOME/homebase/scan.json, or the platform's state dir under $HOME.
func defaultUsageState() string {
	if x := os.Getenv("XDG_STATE_HOME"); x != "" {
		return filepath.Join(x, "homebase", "scan.json")
	}
	home, _ := os.UserHomeDir()
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Application Support", "homebase", "scan.json")
	}
	return filepath.Join(home, ".local", "state", "homebase", "scan.json")
}

// scanFunc scans local agent sessions, keeping the file-offset cache in statePath.
func scanFunc(machine, statePath string) func(context.Context) ([]model.Usage, error) {
	return func(context.Context) ([]model.Usage, error) {
		if err := os.MkdirAll(filepath.Dir(statePath), 0o700); err != nil {
			return nil, err
		}
		return tokens.Scan(tokens.Options{Machine: machine, StatePath: statePath})
	}
}

func usageKey(u model.Usage) string {
	return fmt.Sprintf("%s|%s|%s|%d", u.Machine, u.Agent, u.Model, u.Hour.Unix())
}

func usageHash(u model.Usage) uint64 {
	h := fnv.New64a()
	fmt.Fprintf(h, "%d|%d|%d|%d|%d|%d", u.Input, u.Output, u.CacheRead, u.CacheWrite, u.Reasoning, u.Sessions)
	return h.Sum64()
}

// usageDue picks the rows to send: all of them on the first round, then recent hours and
// anything that changed since the hub last accepted it.
func (a *agent) usageDue(rows []model.Usage) []model.Usage {
	if !a.usageFirst {
		return rows
	}
	cut := a.now().Add(-usageWindow)
	var out []model.Usage
	for _, r := range rows {
		if r.Hour.After(cut) || a.usageSent[usageKey(r)] != usageHash(r) {
			out = append(out, r)
		}
	}
	return out
}

// usageRound scans local sessions and posts the due rows in chunks of at most 2000. A
// transient failure stops the round and leaves the unsent rows due for the next one.
func (a *agent) usageRound(ctx context.Context) error {
	rows, err := a.scan(ctx)
	if err != nil {
		return fmt.Errorf("scan: %w", err)
	}
	if a.usageSent == nil {
		a.usageSent = map[string]uint64{}
	}
	due := a.usageDue(rows)
	var refused error
	for len(due) > 0 {
		n := min(len(due), usageChunk)
		chunk := due[:n]
		due = due[n:]
		body, err := json.Marshal(model.UsageBatch{Machine: a.machine, Rows: chunk})
		if err != nil {
			return err
		}
		if _, err := a.send(ctx, "/api/ingest/usage", body); err != nil {
			if !isPermanent(err) {
				return err
			}
			refused = err // the hub refused this chunk's content; skip it and carry on
		}
		for _, r := range chunk {
			a.usageSent[usageKey(r)] = usageHash(r)
		}
	}
	a.usageFirst = true
	return refused
}

// usageLoop runs usageRound every usageEvery until ctx ends. It runs apart from the probe
// loop so a slow first scan never delays latency samples.
func (a *agent) usageLoop(ctx context.Context) {
	for {
		if err := a.usageRound(ctx); err != nil && ctx.Err() == nil {
			fmt.Fprintf(a.log, "homebase agent: usage post failed, will retry: %v\n", err)
		}
		if a.sleep(ctx, a.usageEvery) != nil {
			return
		}
	}
}

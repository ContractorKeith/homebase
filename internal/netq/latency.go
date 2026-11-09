package netq

import (
	"context"
	"time"

	"github.com/ContractorKeith/homebase/internal/model"
)

// pingTimeout bounds one ping run (10 probes, 1 s reply wait each, plus slack).
const pingTimeout = 20 * time.Second

// Internet is the public latency target.
const Internet = "1.1.1.1"

// Prober measures latency to the default gateway and 1.1.1.1. Fields are injectable for tests.
type Prober struct {
	Machine string
	GOOS    string
	Run     Runner
	Now     func() time.Time
}

// Probe returns one sample per target. A target whose ping is missing or fails yields a
// sample with nil values. An unknown gateway skips that target.
func (p Prober) Probe(ctx context.Context) []model.NetSample {
	type target struct{ name, host string }
	var targets []target
	if gw := Gateway(ctx, p.Run, p.GOOS); gw != "" {
		targets = append(targets, target{"gateway", gw})
	}
	targets = append(targets, target{Internet, Internet})

	var samples []model.NetSample
	for _, t := range targets {
		samples = append(samples, p.one(ctx, t.name, t.host))
	}
	return samples
}

func (p Prober) one(ctx context.Context, name, host string) model.NetSample {
	s := model.NetSample{Machine: p.Machine, Target: name, TS: p.Now().UTC()}
	pctx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	out, _ := p.Run(pctx, "ping", pingArgs(p.GOOS, host)...)
	r := ParsePing(out)
	s.RTTMs, s.JitterMs, s.LossPct = r.RTTMs, r.JitterMs, r.LossPct
	return s
}

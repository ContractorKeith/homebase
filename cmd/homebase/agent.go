package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/ContractorKeith/homebase/internal/machineid"
	"github.com/ContractorKeith/homebase/internal/netq"
)

func init() { commands["agent"] = runAgent }

// runAgent parses flags, builds the agent and runs it until interrupted (or once).
func runAgent(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("agent", flag.ContinueOnError)
	fs.SetOutput(stderr)
	hub := fs.String("hub", "", "hub base URL, e.g. http://studio:7780 (required)")
	keyFile := fs.String("key-file", "", "file holding the shared key (required)")
	interval := fs.Duration("interval", 30*time.Second, "latency probe interval")
	speedEvery := fs.Duration("speed-every", 6*time.Hour, "speed test interval")
	noSpeed := fs.Bool("no-speedtest", false, "never run speed tests")
	usageEvery := fs.Duration("usage-every", 5*time.Minute, "token usage post interval")
	noUsage := fs.Bool("no-usage", false, "never scan or post token usage")
	usageState := fs.String("usage-state", defaultUsageState(), "token scan state file")
	once := fs.Bool("once", false, "run one round, then exit")
	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if *hub == "" || *keyFile == "" || *interval <= 0 || *speedEvery <= 0 || *usageEvery <= 0 {
		fmt.Fprintln(stderr, "homebase agent: --hub and --key-file are required; intervals must be positive")
		return 2
	}
	key, err := os.ReadFile(*keyFile)
	if err != nil || strings.TrimSpace(string(key)) == "" {
		fmt.Fprintf(stderr, "homebase agent: cannot read a key from %s\n", *keyFile)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	machine := machineid.ID(ctx)
	a := &agent{
		hub:        strings.TrimRight(*hub, "/"),
		key:        strings.TrimSpace(string(key)),
		machine:    machine,
		client:     &http.Client{Timeout: 15 * time.Second},
		probe:      netq.Prober{Machine: machine, GOOS: runtime.GOOS, Run: netq.ExecRunner, Now: time.Now}.Probe,
		speed:      func(ctx context.Context) speedResult { return netq.Speed{}.Run(ctx, machine) },
		noSpeed:    *noSpeed,
		interval:   *interval,
		speedEvery: *speedEvery,
		sleep:      sleepCtx,
		now:        time.Now,
		log:        stderr,
		usageEvery: *usageEvery,
	}
	if !*noUsage {
		a.scan = scanFunc(machine, *usageState)
	}
	if *once {
		if err := a.round(ctx, !a.noSpeed); err != nil {
			fmt.Fprintf(stderr, "homebase agent: hub unreachable: %v\n", err)
			return 1
		}
		if a.scan != nil {
			if err := a.usageRound(ctx); err != nil {
				fmt.Fprintf(stderr, "homebase agent: usage post failed: %v\n", err)
				return 1
			}
		}
		fmt.Fprintf(stdout, "homebase agent: posted to %s as %s\n", a.hub, machine)
		return 0
	}
	a.loop(ctx)
	return 0
}

// loop probes every interval and runs a speed test every speedEvery, until ctx ends.
// A failed post keeps its samples in memory; the next round resends them.
func (a *agent) loop(ctx context.Context) {
	fmt.Fprintf(a.log, "homebase agent: %s -> %s every %s\n", a.machine, a.hub, a.interval)
	if a.scan != nil {
		go a.usageLoop(ctx)
	}
	var lastSpeed time.Time
	for {
		due := !a.noSpeed && a.now().Sub(lastSpeed) >= a.speedEvery
		if due {
			lastSpeed = a.now()
		}
		if err := a.round(ctx, due); err != nil && ctx.Err() == nil {
			fmt.Fprintf(a.log, "homebase agent: hub unreachable, holding %d samples: %v\n", len(a.pending.Samples), err)
		}
		if a.sleep(ctx, a.interval) != nil {
			return
		}
	}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

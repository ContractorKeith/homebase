package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/ContractorKeith/homebase/internal/model"
	"github.com/ContractorKeith/homebase/internal/tokens"
)

func init() { commands["scan"] = runScan }

// runScan parses local agent sessions and prints hourly rows (--json) or a totals table.
func runScan(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	fs.SetOutput(stderr)
	agent := fs.String("agent", "", "claude, codex or grok (default all)")
	asJSON := fs.Bool("json", false, "print hourly rows as JSON")
	state := fs.String("state", "", "state file that keeps totals when session files are deleted")
	home := fs.String("home", "", "home directory to read agent stores from (default $HOME)")
	machine := fs.String("machine", "", "machine id stamped on rows")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	opts := tokens.Options{Home: *home, StatePath: *state, Machine: *machine}
	switch *agent {
	case "":
	case model.AgentClaude, model.AgentCodex, model.AgentGrok:
		opts.Agents = []string{*agent}
	default:
		fmt.Fprintf(stderr, "homebase scan: unknown --agent %q (claude, codex or grok)\n", *agent)
		return 2
	}
	rows, err := tokens.Scan(opts)
	if err != nil {
		fmt.Fprintln(stderr, "homebase scan:", err)
		return 1
	}
	if *asJSON {
		if rows == nil {
			rows = []model.Usage{}
		}
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(rows); err != nil {
			fmt.Fprintln(stderr, "homebase scan:", err)
			return 1
		}
		return 0
	}
	printScanTable(stdout, rows)
	return 0
}

// printScanTable prints totals per agent and model.
func printScanTable(w io.Writer, rows []model.Usage) {
	type tot struct{ in, out, cr, cw, re int64 }
	sums := map[string]*tot{}
	for _, r := range rows {
		k := r.Agent + "\x00" + r.Model
		t := sums[k]
		if t == nil {
			t = &tot{}
			sums[k] = t
		}
		t.in, t.out, t.cr, t.cw, t.re = t.in+r.Input, t.out+r.Output, t.cr+r.CacheRead, t.cw+r.CacheWrite, t.re+r.Reasoning
	}
	keys := make([]string, 0, len(sums))
	for k := range sums {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	fmt.Fprintf(w, "%-8s %-28s %12s %12s %12s %12s %12s\n", "AGENT", "MODEL", "INPUT", "OUTPUT", "CACHE READ", "CACHE WRITE", "REASONING")
	for _, k := range keys {
		p := strings.SplitN(k, "\x00", 2)
		t := sums[k]
		fmt.Fprintf(w, "%-8s %-28s %12d %12d %12d %12d %12d\n", p[0], p[1], t.in, t.out, t.cr, t.cw, t.re)
	}
}

// Package netq measures network quality: latency, jitter and loss with the system ping,
// and download/upload speed against Cloudflare. Unknown values are nil, never zero.
package netq

import (
	"context"
	"os/exec"
	"regexp"
	"strconv"
)

// Runner runs a command and returns its combined output. A non-zero exit still returns
// the output: ping exits 1 on packet loss but its summary is what we want.
type Runner func(ctx context.Context, name string, args ...string) (string, error)

// ExecRunner is the real Runner.
func ExecRunner(ctx context.Context, name string, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	return string(out), err
}

// pingArgs returns the unprivileged ping arguments: 10 probes 0.2 s apart. Linux iputils
// -W is seconds; BSD/macOS -W is milliseconds.
func pingArgs(goos, host string) []string {
	if goos == "darwin" {
		return []string{"-n", "-c", "10", "-i", "0.2", "-W", "1000", host}
	}
	return []string{"-n", "-c", "10", "-i", "0.2", "-W", "1", host}
}

// Result is one parsed ping run; nil fields are unknown.
type Result struct {
	RTTMs, JitterMs, LossPct *float64
}

var (
	lossRe = regexp.MustCompile(`([\d.]+)% packet loss`)
	// Linux: "rtt min/avg/max/mdev = ..."; macOS: "round-trip min/avg/max/stddev = ...".
	rttRe = regexp.MustCompile(`min/avg/max/(?:mdev|stddev) = [\d.]+/([\d.]+)/[\d.]+/([\d.]+) ms`)
)

// ParsePing reads iputils or BSD ping output. No summary line (unreachable, unknown host)
// gives all nil. 100% loss gives loss only.
func ParsePing(out string) Result {
	var r Result
	if m := lossRe.FindStringSubmatch(out); m != nil {
		r.LossPct = num(m[1])
	}
	if m := rttRe.FindStringSubmatch(out); m != nil {
		r.RTTMs, r.JitterMs = num(m[1]), num(m[2])
	}
	return r
}

func num(s string) *float64 {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return &f
}

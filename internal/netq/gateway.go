package netq

import (
	"context"
	"net/netip"
	"regexp"
	"strings"
)

var (
	ipRouteRe  = regexp.MustCompile(`(?m)^default via (\S+)`)
	routeGetRe = regexp.MustCompile(`(?m)^\s*gateway:\s*(\S+)`)
)

// Gateway returns the default IPv4 gateway, or "" when unknown (no route, v6-only, or the
// command is missing).
func Gateway(ctx context.Context, run Runner, goos string) string {
	var out string
	var re *regexp.Regexp
	if goos == "darwin" {
		out, _ = run(ctx, "route", "-n", "get", "default")
		re = routeGetRe
	} else {
		out, _ = run(ctx, "ip", "route", "show", "default")
		re = ipRouteRe
	}
	return parseGateway(out, re)
}

func parseGateway(out string, re *regexp.Regexp) string {
	m := re.FindStringSubmatch(out)
	if m == nil {
		return ""
	}
	addr, err := netip.ParseAddr(strings.TrimSpace(m[1]))
	if err != nil || !addr.Is4() {
		return ""
	}
	return addr.String()
}

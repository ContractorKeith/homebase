// Package machineid derives the id machine-monitor reports for this machine, so homebase
// rows join to its cards. The rule matches machine-monitor's collectors exactly.
package machineid

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// Runner runs a command and returns its stdout.
type Runner func(ctx context.Context, name string, args ...string) (string, error)

// Source is where ids are read from; tests swap every field.
type Source struct {
	GOOS     string                 // "linux" or "darwin"
	Root     string                 // filesystem root for /etc/machine-id
	Run      Runner                 // runs ioreg and scutil
	Hostname func() (string, error) // last-resort fallback
}

// ID returns this machine's id using the real system.
func ID(ctx context.Context) string {
	return Source{GOOS: runtime.GOOS, Root: "/", Run: execRun, Hostname: os.Hostname}.ID(ctx)
}

func execRun(ctx context.Context, name string, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, name, args...).Output()
	return string(out), err
}

// ID applies the per-OS rule. Linux: /etc/machine-id, /var/lib/dbus/machine-id, hostname.
// macOS: IOPlatformUUID, then ComputerName (then the short hostname).
func (s Source) ID(ctx context.Context) string {
	if s.GOOS == "darwin" {
		return s.darwin(ctx)
	}
	for _, p := range []string{"etc/machine-id", "var/lib/dbus/machine-id"} {
		if id := s.firstLine(p); id != "" {
			return id
		}
	}
	h, _ := s.Hostname()
	return h
}

func (s Source) darwin(ctx context.Context) string {
	if out, err := s.Run(ctx, "/usr/sbin/ioreg", "-rd1", "-c", "IOPlatformExpertDevice"); err == nil {
		if m := uuidRe.FindStringSubmatch(out); m != nil {
			return m[1]
		}
	}
	if out, err := s.Run(ctx, "/usr/sbin/scutil", "--get", "ComputerName"); err == nil {
		if name := firstLine(out); name != "" {
			return name
		}
	}
	h, _ := s.Hostname()
	short, _, _ := strings.Cut(h, ".") // hostname -s
	return short
}

var uuidRe = regexp.MustCompile(`"IOPlatformUUID"\s*=\s*"([^"]+)"`)

func (s Source) firstLine(rel string) string {
	b, err := os.ReadFile(filepath.Join(s.Root, rel))
	if err != nil {
		return ""
	}
	return firstLine(string(b))
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return strings.TrimSpace(line)
}

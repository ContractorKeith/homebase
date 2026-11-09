// Package service installs homebase as a per-user service: a systemd user unit on Linux
// and a LaunchAgent on macOS. Neither needs admin rights.
package service

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Runner runs a service-manager command and returns its combined output.
// Tests replace it with a fake; they never call the real systemctl or launchctl.
type Runner func(name string, args ...string) ([]byte, error)

// ExecRunner runs the real command.
func ExecRunner(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

// Manager renders and installs one service. Everything that touches the machine is a field.
type Manager struct {
	GOOS   string // "linux" or "darwin"
	Home   string
	UID    int
	Exe    string // absolute path of the binary, symlinks resolved
	Run    Runner
	Exists func(path string) bool // reports whether a path exists (mise shims check)
	Out    io.Writer
	DryRun bool // print files and commands, change nothing
}

// Spec is one service: the role (hub or agent) and the flags passed to that subcommand.
type Spec struct {
	Role string
	Args []string
}

// Valid reports whether r is a role homebase can install.
func Valid(role string) bool { return role == "hub" || role == "agent" }

// ExpandHome replaces a leading ~ in each arg, including after "=" in --flag=~/x.
func ExpandHome(home string, args []string) []string {
	out := make([]string, len(args))
	for i, a := range args {
		flag, val := "", a
		if k := strings.Index(a, "=~"); strings.HasPrefix(a, "-") && k > 0 {
			flag, val = a[:k+1], a[k+1:]
		}
		if val == "~" || strings.HasPrefix(val, "~/") {
			val = filepath.Join(home, val[1:])
		}
		out[i] = flag + val
	}
	return out
}

// pathValue is the PATH the service runs with: enough for the hub to find gh and the agent
// to find ping. Linux adds the mise shims dir only when it exists.
func (m *Manager) pathValue() string {
	var dirs []string
	if shims := filepath.Join(m.Home, ".local", "share", "mise", "shims"); m.GOOS == "linux" && m.Exists(shims) {
		dirs = append(dirs, shims)
	}
	dirs = append(dirs, "/opt/homebrew/bin", "/usr/local/bin", filepath.Join(m.Home, ".local", "bin"),
		"/usr/bin", "/bin", "/usr/sbin", "/sbin")
	return strings.Join(dirs, ":")
}

func (m *Manager) unitName(role string) string { return "homebase-" + role + ".service" }
func (m *Manager) label(role string) string    { return "com.contractorkeith.homebase." + role }

// FilePath is where the unit or plist lives.
func (m *Manager) FilePath(role string) string {
	if m.GOOS == "darwin" {
		return filepath.Join(m.Home, "Library", "LaunchAgents", m.label(role)+".plist")
	}
	return filepath.Join(m.Home, ".config", "systemd", "user", m.unitName(role))
}

// logPath is the macOS log file for a role and stream ("out" or "err").
func (m *Manager) logPath(role, stream string) string {
	return filepath.Join(m.Home, "Library", "Logs", "homebase", role+"."+stream+".log")
}

// Render returns the unit or plist text for s.
func (m *Manager) Render(s Spec) string {
	if m.GOOS == "darwin" {
		return m.plist(s)
	}
	return m.unit(s)
}

// unit renders the systemd user unit.
func (m *Manager) unit(s Spec) string {
	words := append([]string{m.Exe, s.Role}, s.Args...)
	for i, w := range words {
		words[i] = systemdQuote(w)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "[Unit]\nDescription=homebase %s\n\n[Service]\n", s.Role)
	fmt.Fprintf(&b, "ExecStart=%s\n", strings.Join(words, " "))
	fmt.Fprintf(&b, "Environment=%s\n", systemdQuote("PATH="+m.pathValue()))
	b.WriteString("Restart=on-failure\nRestartSec=10\n\n[Install]\nWantedBy=default.target\n")
	return b.String()
}

// systemdQuote double-quotes one word, escaping what systemd would expand (\, ", % and $).
func systemdQuote(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "%", "%%", "$", "$$", "\n", `\n`)
	return `"` + r.Replace(s) + `"`
}

// plist renders the LaunchAgent.
func (m *Manager) plist(s Spec) string {
	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	b.WriteString("<!DOCTYPE plist PUBLIC \"-//Apple//DTD PLIST 1.0//EN\" \"http://www.apple.com/DTDs/PropertyList-1.0.dtd\">\n")
	b.WriteString("<plist version=\"1.0\">\n<dict>\n")
	fmt.Fprintf(&b, "\t<key>Label</key>\n\t<string>%s</string>\n", xmlEscape(m.label(s.Role)))
	b.WriteString("\t<key>ProgramArguments</key>\n\t<array>\n")
	for _, w := range append([]string{m.Exe, s.Role}, s.Args...) {
		fmt.Fprintf(&b, "\t\t<string>%s</string>\n", xmlEscape(w))
	}
	b.WriteString("\t</array>\n")
	fmt.Fprintf(&b, "\t<key>EnvironmentVariables</key>\n\t<dict>\n\t\t<key>PATH</key>\n\t\t<string>%s</string>\n\t</dict>\n", xmlEscape(m.pathValue()))
	b.WriteString("\t<key>RunAtLoad</key>\n\t<true/>\n\t<key>KeepAlive</key>\n\t<true/>\n")
	fmt.Fprintf(&b, "\t<key>StandardOutPath</key>\n\t<string>%s</string>\n", xmlEscape(m.logPath(s.Role, "out")))
	fmt.Fprintf(&b, "\t<key>StandardErrorPath</key>\n\t<string>%s</string>\n", xmlEscape(m.logPath(s.Role, "err")))
	b.WriteString("</dict>\n</plist>\n")
	return b.String()
}

func xmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}

// Default returns a Manager for the running machine, with the binary path pinned to the
// running executable resolved through symlinks.
func Default(out io.Writer) (*Manager, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return nil, err
	}
	return &Manager{GOOS: goos(), Home: home, UID: os.Getuid(), Exe: exe, Run: ExecRunner, Out: out,
		Exists: func(p string) bool { _, err := os.Stat(p); return err == nil }}, nil
}

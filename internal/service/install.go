package service

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

func goos() string { return runtime.GOOS }

// run runs one command, or just prints it on a dry run.
func (m *Manager) run(name string, args ...string) ([]byte, error) {
	fmt.Fprintf(m.Out, "$ %s %s\n", name, strings.Join(args, " "))
	if m.DryRun {
		return nil, nil
	}
	return m.Run(name, args...)
}

// step runs a command that must succeed.
func (m *Manager) step(name string, args ...string) error {
	if out, err := m.run(name, args...); err != nil {
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (m *Manager) domain() string { return "gui/" + strconv.Itoa(m.UID) }

// Install writes the unit or plist and starts the service. A reinstall replaces it.
func (m *Manager) Install(s Spec) error {
	if !Valid(s.Role) {
		return fmt.Errorf("unknown role %q", s.Role)
	}
	path, text := m.FilePath(s.Role), m.Render(s)
	if m.DryRun {
		fmt.Fprintf(m.Out, "# would write %s\n%s", path, text)
	} else {
		dirs := []string{filepath.Dir(path)}
		if m.GOOS == "darwin" {
			dirs = append(dirs, filepath.Dir(m.logPath(s.Role, "out")))
		}
		for _, d := range dirs {
			if err := os.MkdirAll(d, 0o755); err != nil {
				return err
			}
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			return err
		}
	}
	if m.GOOS == "darwin" {
		_, _ = m.run("launchctl", "bootout", m.domain()+"/"+m.label(s.Role)) // not loaded yet is fine
		return m.step("launchctl", "bootstrap", m.domain(), path)
	}
	if err := m.step("systemctl", "--user", "daemon-reload"); err != nil {
		return err
	}
	return m.step("systemctl", "--user", "enable", "--now", m.unitName(s.Role))
}

// Uninstall stops the service and removes its file.
func (m *Manager) Uninstall(role string) error {
	if !Valid(role) {
		return fmt.Errorf("unknown role %q", role)
	}
	path := m.FilePath(role)
	if m.GOOS == "darwin" {
		_, _ = m.run("launchctl", "bootout", m.domain()+"/"+m.label(role))
	} else {
		_, _ = m.run("systemctl", "--user", "disable", "--now", m.unitName(role))
	}
	if m.DryRun {
		fmt.Fprintf(m.Out, "# would remove %s\n", path)
	} else if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	if m.GOOS != "darwin" {
		return m.step("systemctl", "--user", "daemon-reload")
	}
	return nil
}

// Status prints whether the service is installed and running.
func (m *Manager) Status(role string) error {
	path := m.FilePath(role)
	if _, err := os.Stat(path); err != nil {
		fmt.Fprintf(m.Out, "homebase %s: not installed\n", role)
		return nil
	}
	var running bool
	if m.GOOS == "darwin" {
		out, err := m.Run("launchctl", "print", m.domain()+"/"+m.label(role))
		running = err == nil && strings.Contains(string(out), "state = running")
	} else {
		out, _ := m.Run("systemctl", "--user", "is-active", m.unitName(role))
		running = strings.TrimSpace(string(out)) == "active"
	}
	state := "not running"
	if running {
		state = "running"
	}
	fmt.Fprintf(m.Out, "homebase %s: installed (%s), %s\n", role, path, state)
	return nil
}

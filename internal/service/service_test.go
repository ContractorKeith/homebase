package service

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files")

type call struct{ name, args string }

// fake builds a Manager over a temp home whose runner records commands instead of running them.
func fake(t *testing.T, goos string, shims bool) (*Manager, *[]call, *bytes.Buffer) {
	t.Helper()
	var calls []call
	out := &bytes.Buffer{}
	return &Manager{
		GOOS: goos, Home: t.TempDir(), UID: 501, Exe: "/opt/hb/homebase", Out: out,
		Run: func(name string, args ...string) ([]byte, error) {
			calls = append(calls, call{name, strings.Join(args, " ")})
			return nil, nil
		},
		Exists: func(string) bool { return shims },
	}, &calls, out
}

var agentSpec = Spec{Role: "agent", Args: []string{"--hub", "http://mac-studio:7780", "--key-file", "/home/k/.config/homebase/key"}}

func golden(t *testing.T, name, got string) {
	t.Helper()
	p := filepath.Join("..", "..", "testdata", "service", name)
	if *update {
		if err := os.WriteFile(p, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("%s differs from golden:\n%s", name, got)
	}
}

func TestRenderGolden(t *testing.T) {
	m, _, _ := fake(t, "linux", true)
	m.Home = "/home/k"
	golden(t, "homebase-agent.service", m.Render(agentSpec))
	m.GOOS = "darwin"
	m.Home = "/Users/k"
	golden(t, "com.contractorkeith.homebase.hub.plist", m.Render(Spec{Role: "hub", Args: []string{"--mm-key-file", "/Users/k/mm.key"}}))
}

func TestMiseShimsOnlyOnLinuxWhenPresent(t *testing.T) {
	for _, c := range []struct {
		goos  string
		shims bool
		want  bool
	}{{"linux", true, true}, {"linux", false, false}, {"darwin", true, false}} {
		m, _, _ := fake(t, c.goos, c.shims)
		if got := strings.Contains(m.pathValue(), "mise/shims"); got != c.want {
			t.Errorf("%s shims=%v: got %v", c.goos, c.shims, got)
		}
	}
}

func TestInstallUninstallLinux(t *testing.T) {
	m, calls, _ := fake(t, "linux", false)
	if err := m.Install(agentSpec); err != nil {
		t.Fatal(err)
	}
	want := []call{{"systemctl", "--user daemon-reload"}, {"systemctl", "--user enable --now homebase-agent.service"}}
	if !reflect.DeepEqual(*calls, want) {
		t.Errorf("install calls = %v", *calls)
	}
	if _, err := os.Stat(m.FilePath("agent")); err != nil {
		t.Fatal(err)
	}
	*calls = nil
	if err := m.Uninstall("agent"); err != nil {
		t.Fatal(err)
	}
	want = []call{{"systemctl", "--user disable --now homebase-agent.service"}, {"systemctl", "--user daemon-reload"}}
	if !reflect.DeepEqual(*calls, want) {
		t.Errorf("uninstall calls = %v", *calls)
	}
	if _, err := os.Stat(m.FilePath("agent")); !os.IsNotExist(err) {
		t.Errorf("unit still there: %v", err)
	}
}

func TestInstallUninstallMac(t *testing.T) {
	m, calls, _ := fake(t, "darwin", false)
	if err := m.Install(Spec{Role: "hub"}); err != nil {
		t.Fatal(err)
	}
	file := m.FilePath("hub")
	want := []call{
		{"launchctl", "bootout gui/501/com.contractorkeith.homebase.hub"},
		{"launchctl", "bootstrap gui/501 " + file},
	}
	if !reflect.DeepEqual(*calls, want) {
		t.Errorf("install calls = %v", *calls)
	}
	if _, err := os.Stat(filepath.Join(m.Home, "Library", "Logs", "homebase")); err != nil {
		t.Errorf("log dir missing: %v", err)
	}
	*calls = nil
	if err := m.Uninstall("hub"); err != nil {
		t.Fatal(err)
	}
	if want := []call{{"launchctl", "bootout gui/501/com.contractorkeith.homebase.hub"}}; !reflect.DeepEqual(*calls, want) {
		t.Errorf("uninstall calls = %v", *calls)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Errorf("plist still there: %v", err)
	}
}

func TestDryRunChangesNothing(t *testing.T) {
	m, calls, out := fake(t, "linux", false)
	m.DryRun = true
	if err := m.Install(agentSpec); err != nil {
		t.Fatal(err)
	}
	if len(*calls) != 0 {
		t.Errorf("dry run ran %v", *calls)
	}
	if _, err := os.Stat(m.FilePath("agent")); !os.IsNotExist(err) {
		t.Error("dry run wrote the unit")
	}
	if !strings.Contains(out.String(), "ExecStart=") || !strings.Contains(out.String(), "$ systemctl --user daemon-reload") {
		t.Errorf("dry run output:\n%s", out)
	}
}

func TestStatus(t *testing.T) {
	m, _, out := fake(t, "linux", false)
	_ = m.Status("hub")
	if !strings.Contains(out.String(), "not installed") {
		t.Errorf("got %q", out)
	}
	_ = m.Install(Spec{Role: "hub"})
	m.Run = func(string, ...string) ([]byte, error) { return []byte("active\n"), nil }
	out.Reset()
	_ = m.Status("hub")
	if !strings.Contains(out.String(), "installed") || !strings.HasSuffix(strings.TrimSpace(out.String()), "running") {
		t.Errorf("got %q", out)
	}
}

func TestExpandHome(t *testing.T) {
	got := ExpandHome("/h", []string{"--key-file", "~/k", "--key-file=~/k", "~", "http://x/~y", "a~"})
	want := []string{"--key-file", "/h/k", "--key-file=/h/k", "/h", "http://x/~y", "a~"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v", got)
	}
}

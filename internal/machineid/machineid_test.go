package machineid

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

const ioregOut = `+-o Root  <class IORegistryEntry, id 0x100000100, retain 29>
  +-o Mac Studio  <class IOPlatformExpertDevice, id 0x10000020f, registered, matched, active, busy 0 (1090 ms), retain 40>
    {
      "IOPlatformSerialNumber" = "ABC123"
      "IOPlatformUUID" = "9F1C2A3B-4D5E-6F70-8192-A3B4C5D6E7F8"
      "model" = <"Mac13,2">
    }
`

func fakeRun(outs map[string]string) Runner {
	return func(_ context.Context, name string, _ ...string) (string, error) {
		if o, ok := outs[name]; ok {
			return o, nil
		}
		return "", errors.New("not found")
	}
}

func host(h string) func() (string, error) { return func() (string, error) { return h, nil } }

func writeRoot(t *testing.T, files map[string]string) string {
	root := t.TempDir()
	for p, c := range files {
		full := filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestLinux(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"etc", map[string]string{"etc/machine-id": "abc123\n", "var/lib/dbus/machine-id": "zzz\n"}, "abc123"},
		{"dbus", map[string]string{"var/lib/dbus/machine-id": "dbus1\n"}, "dbus1"},
		{"blank etc falls through", map[string]string{"etc/machine-id": "\n", "var/lib/dbus/machine-id": "dbus1"}, "dbus1"},
		{"hostname", nil, "omarchy"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := Source{GOOS: "linux", Root: writeRoot(t, c.files), Hostname: host("omarchy")}
			if got := s.ID(context.Background()); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestDarwin(t *testing.T) {
	cases := []struct {
		name string
		outs map[string]string
		want string
	}{
		{"uuid", map[string]string{"/usr/sbin/ioreg": ioregOut, "/usr/sbin/scutil": "Studio\n"}, "9F1C2A3B-4D5E-6F70-8192-A3B4C5D6E7F8"},
		{"computer name", map[string]string{"/usr/sbin/ioreg": "garbage", "/usr/sbin/scutil": "Mac Studio\n"}, "Mac Studio"},
		{"short hostname", nil, "studio"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := Source{GOOS: "darwin", Root: t.TempDir(), Run: fakeRun(c.outs), Hostname: host("studio.local")}
			if got := s.ID(context.Background()); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

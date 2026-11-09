package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestScanBadFlags(t *testing.T) {
	for _, args := range [][]string{{"scan", "--agent", "nope"}, {"scan", "--bogus"}} {
		if rc := run(args, &bytes.Buffer{}, &bytes.Buffer{}); rc != 2 {
			t.Errorf("%v: exit %d, want 2", args, rc)
		}
	}
}

func TestScanJSONEmptyHome(t *testing.T) {
	var out bytes.Buffer
	if rc := run([]string{"scan", "--json", "--home", t.TempDir()}, &out, &bytes.Buffer{}); rc != 0 {
		t.Fatalf("exit %d", rc)
	}
	if strings.TrimSpace(out.String()) != "[]" {
		t.Errorf("out = %q", out.String())
	}
}

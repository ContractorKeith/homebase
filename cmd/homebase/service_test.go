package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestServiceUsageErrors(t *testing.T) {
	for _, args := range [][]string{
		{"service"},
		{"service", "install", "--role", "nope", "--dry-run"},
		{"service", "install", "--dry-run"},
		{"service", "frob", "--role", "hub"},
		{"service", "install", "--role", "agent", "--dry-run"}, // agent needs --hub and --key-file
	} {
		if code := run(args, &bytes.Buffer{}, &bytes.Buffer{}); code != 2 {
			t.Errorf("%v: exit %d, want 2", args, code)
		}
	}
}

func TestServiceDryRunAgent(t *testing.T) {
	var out bytes.Buffer
	args := []string{"service", "install", "--role", "agent", "--dry-run", "--", "--hub", "http://s:7780", "--key-file", "~/k"}
	if code := run(args, &out, &bytes.Buffer{}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out.String(), "enable --now") && !strings.Contains(out.String(), "bootstrap") {
		t.Errorf("no install command in:\n%s", out.String())
	}
	if strings.Contains(out.String(), "~/k") {
		t.Errorf("~ not expanded:\n%s", out.String())
	}
}

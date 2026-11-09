package main

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrCreateKey(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "key")
	k1, err := loadOrCreateKey(p, io.Discard)
	if err != nil || len(k1) < 32 {
		t.Fatalf("create: %q %v", k1, err)
	}
	if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", fi.Mode().Perm())
	}
	if k2, _ := loadOrCreateKey(p, io.Discard); k2 != k1 {
		t.Error("an existing key must be reused")
	}
}

func TestHubEnvDefault(t *testing.T) {
	t.Setenv("HOMEBASE_X", "from-env")
	if envOr("HOMEBASE_X", "d") != "from-env" || envOr("HOMEBASE_UNSET_Y", "d") != "d" {
		t.Error("envOr")
	}
}

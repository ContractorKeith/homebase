package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/ContractorKeith/homebase/internal/hub"
	"github.com/ContractorKeith/homebase/internal/mm"
	"github.com/ContractorKeith/homebase/internal/store"
)

func init() { commands["hub"] = runHub }

// envOr returns $name when set, else def. Flags then override both.
func envOr(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

// defaultDBPath is $XDG_DATA_HOME/homebase/hub.db, or the platform's data dir under $HOME.
func defaultDBPath() string {
	if x := os.Getenv("XDG_DATA_HOME"); x != "" {
		return filepath.Join(x, "homebase", "hub.db")
	}
	home, _ := os.UserHomeDir()
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Application Support", "homebase", "hub.db")
	}
	return filepath.Join(home, ".local", "share", "homebase", "hub.db")
}

func defaultKeyPath() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "homebase", "key")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "homebase", "key")
}

// loadOrCreateKey reads the key file, or generates a random key and writes it 0600.
func loadOrCreateKey(path string, stderr io.Writer) (string, error) {
	b, err := os.ReadFile(path)
	if err == nil {
		if k := strings.TrimSpace(string(b)); k != "" {
			return k, nil
		}
		return "", fmt.Errorf("%s is empty", path)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	key := hex.EncodeToString(raw)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(key+"\n"), 0o600); err != nil {
		return "", err
	}
	fmt.Fprintf(stderr, "homebase hub: generated a new key in %s\n", path)
	return key, nil
}

// runHub serves the dashboard: auth, the machine routes and the embedded web UI.
func runHub(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("hub", flag.ContinueOnError)
	fs.SetOutput(stderr)
	listen := fs.String("listen", envOr("HOMEBASE_LISTEN", ":7780"), "address to listen on")
	mmURL := fs.String("mm", envOr("HOMEBASE_MM", "http://localhost:7777"), "machine-monitor hub URL")
	mmKeyFile := fs.String("mm-key-file", envOr("HOMEBASE_MM_KEY_FILE", ""), "file holding the machine-monitor key")
	keyFile := fs.String("key-file", envOr("HOMEBASE_KEY_FILE", defaultKeyPath()), "homebase key file (created if missing)")
	dbPath := fs.String("db", envOr("HOMEBASE_DB", defaultDBPath()), "SQLite database path")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}

	key, err := loadOrCreateKey(*keyFile, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "homebase hub: key: %v\n", err)
		return 1
	}
	mmKey := ""
	if *mmKeyFile != "" {
		b, err := os.ReadFile(*mmKeyFile)
		if err != nil {
			fmt.Fprintf(stderr, "homebase hub: machine-monitor key: %v\n", err)
			return 1
		}
		mmKey = strings.TrimSpace(string(b))
	}
	if err := os.MkdirAll(filepath.Dir(*dbPath), 0o700); err != nil {
		fmt.Fprintf(stderr, "homebase hub: db: %v\n", err)
		return 1
	}
	st, err := store.Open(*dbPath)
	if err != nil {
		fmt.Fprintf(stderr, "homebase hub: db: %v\n", err)
		return 1
	}
	defer st.Close()

	logf := func(format string, a ...any) { fmt.Fprintf(stderr, "homebase hub: "+format+"\n", a...) }
	srv := hub.New(hub.Config{Key: key, MM: mm.New(*mmURL, mmKey), Store: st, Logf: logf})
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go srv.Run(ctx)

	httpSrv := &http.Server{Addr: *listen, Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shut, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		httpSrv.Shutdown(shut)
	}()
	fmt.Fprintf(stdout, "homebase hub on %s (key in %s; open /?key=<key>), machine-monitor at %s\n", *listen, *keyFile, *mmURL)
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintf(stderr, "homebase hub: %v\n", err)
		return 1
	}
	return 0
}

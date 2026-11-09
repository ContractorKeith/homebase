// Package hub is the homebase HTTP server: key auth, the embedded web UI and the machine
// routes. Routes are registered in routes.go.
package hub

import (
	"context"
	"log"
	"net/http"

	"github.com/ContractorKeith/homebase/internal/mm"
	"github.com/ContractorKeith/homebase/internal/store"
)

// Source is the machine-monitor client the hub needs; *mm.Client satisfies it.
type Source interface {
	Devices(ctx context.Context) ([]mm.Device, error)
	History(ctx context.Context, id, rng string) (*mm.History, error)
	Stream(ctx context.Context) <-chan mm.Event
}

// Config is what New needs.
type Config struct {
	Key   string       // the shared homebase key; required
	MM    Source       // machine-monitor hub client; required
	Store *store.Store // history database; nil in tests that don't need it (M4 uses it)
	Logf  func(format string, args ...any)
}

// Server is the hub. Create one with New, start Run, and serve Handler.
type Server struct {
	Store *store.Store
	key   string
	mm    Source
	feed  *feed
	speed speedQueue
	logf  func(string, ...any)
}

// New builds a server.
func New(cfg Config) *Server {
	logf := cfg.Logf
	if logf == nil {
		logf = log.Printf
	}
	return &Server{Store: cfg.Store, key: cfg.Key, mm: cfg.MM, feed: newFeed(), logf: logf}
}

// Run follows machine-monitor until ctx ends. Start it once, in its own goroutine.
func (s *Server) Run(ctx context.Context) { s.feed.run(ctx, s.mm) }

// Handler is the full HTTP handler: every route sits behind key auth.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	s.routes(mux)
	return s.auth(mux)
}

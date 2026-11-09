package hub

import (
	"io/fs"
	"net/http"

	"github.com/ContractorKeith/homebase/web"
)

// routes registers every route in one place. M4 and M5 add theirs here (ingest, tokens,
// github) and leave the handlers below alone.
func (s *Server) routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/machines", s.handleMachines)
	mux.HandleFunc("GET /api/stream", s.handleStream)
	mux.HandleFunc("GET /api/machines/{id}/history", s.handleHistory)
	mux.HandleFunc("POST /api/ingest/usage", s.handleIngestUsage)
	mux.HandleFunc("POST /api/ingest/net", s.handleIngestNet)
	mux.HandleFunc("POST /api/speedtest/{id}", s.handleSpeedtest)
	mux.HandleFunc("GET /api/tokens", s.handleTokens)
	mux.HandleFunc("GET /api/github", s.handleGitHub)
	mux.HandleFunc("GET /api/net", s.handleNet)

	static, err := fs.Sub(web.Files, ".")
	if err != nil {
		panic(err) // the embed is fixed at build time
	}
	mux.Handle("GET /", http.FileServerFS(static))
}

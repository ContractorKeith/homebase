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

	static, err := fs.Sub(web.Files, ".")
	if err != nil {
		panic(err) // the embed is fixed at build time
	}
	mux.Handle("GET /", http.FileServerFS(static))
}

package hub

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/ContractorKeith/homebase/internal/mm"
)

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

// mmFail maps a machine-monitor error to our status: its 404 stays 404, anything else is a
// bad gateway. Never a 500.
func mmFail(w http.ResponseWriter, err error) {
	var se *mm.StatusError
	if errors.As(err, &se) && se.Code == http.StatusNotFound {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown machine"})
		return
	}
	writeJSON(w, http.StatusBadGateway, map[string]string{"error": "machine-monitor: " + err.Error()})
}

// handleMachines serves the cards from the live cache, or straight from machine-monitor
// until the stream has delivered its first snapshot.
func (s *Server) handleMachines(w http.ResponseWriter, r *http.Request) {
	if list, ok := s.feed.current(); ok {
		writeJSON(w, http.StatusOK, list)
		return
	}
	devs, err := s.mm.Devices(r.Context())
	if err != nil {
		mmFail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, machines(devs))
}

// historyRanges are the ranges the UI offers; machine-monitor knows more.
var historyRanges = map[string]bool{"1h": true, "24h": true, "7d": true}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	rng := r.URL.Query().Get("range")
	if rng == "" {
		rng = "1h"
	}
	if !historyRanges[rng] {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "range must be 1h, 24h or 7d"})
		return
	}
	h, err := s.mm.History(r.Context(), r.PathValue("id"), rng)
	if err != nil {
		mmFail(w, err)
		return
	}
	if h.Points == nil {
		h.Points = []mm.Point{}
	}
	writeJSON(w, http.StatusOK, h)
}

// handleStream is the browser's SSE feed: a snapshot, then machine, status, removed and mm
// (connection state) messages.
func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming unsupported"})
		return
	}
	first, ch, cancel := s.feed.subscribe()
	defer cancel()
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Write(first)
	fl.Flush()
	beat := time.NewTicker(15 * time.Second)
	defer beat.Stop()
	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return // fell behind: the browser reconnects for a fresh snapshot
			}
			if _, err := w.Write(msg); err != nil {
				return
			}
		case <-beat.C:
			if _, err := w.Write([]byte(": keepalive\n\n")); err != nil {
				return
			}
		case <-r.Context().Done():
			return
		}
		fl.Flush()
	}
}

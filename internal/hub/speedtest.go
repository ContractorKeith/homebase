package hub

import (
	"net/http"
	"sync"
)

// speedQueue remembers which machines were asked for a speed test. The zero value is ready.
type speedQueue struct {
	mu   sync.Mutex
	want map[string]bool
}

func (q *speedQueue) mark(id string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.want == nil {
		q.want = map[string]bool{}
	}
	q.want[id] = true
}

// take reports whether id was marked, and clears the mark so the answer is true once.
func (q *speedQueue) take(id string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	was := q.want[id]
	delete(q.want, id)
	return was
}

// handleSpeedtest queues a "run now" for a machine; its next net post carries the flag.
func (s *Server) handleSpeedtest(w http.ResponseWriter, r *http.Request) {
	s.speed.mark(r.PathValue("id"))
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "queued"})
}

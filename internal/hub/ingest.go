package hub

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/ContractorKeith/homebase/internal/model"
)

const (
	maxIngestBody = 8 << 20
	maxFuture     = time.Hour
)

var earliest = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)

// timeOK reports whether t is plausible: after 2020 and no more than an hour ahead.
func timeOK(t, now time.Time) bool {
	return !t.Before(earliest) && !t.After(now.Add(maxFuture))
}

// decodeBatch reads a size-limited JSON body into v and requires a machine id. It writes
// the error reply itself and returns false when the request is unusable.
func decodeBatch(w http.ResponseWriter, r *http.Request, v any, machine func() string) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxIngestBody)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "body over 8 MB"})
		} else {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad body: " + err.Error()})
		}
		return false
	}
	if machine() == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "machine is required"})
		return false
	}
	return true
}

func validAgent(a string) bool {
	return a == model.AgentClaude || a == model.AgentCodex || a == model.AgentGrok
}

func nonNeg(n int64) int64 {
	if n < 0 {
		return 0
	}
	return n
}

// cleanUsage stamps the batch machine, clamps negatives and drops rows with an unknown
// agent or an implausible hour.
func cleanUsage(b model.UsageBatch, now time.Time) []model.Usage {
	out := make([]model.Usage, 0, len(b.Rows))
	for _, r := range b.Rows {
		if !validAgent(r.Agent) || !timeOK(r.Hour, now) {
			continue
		}
		r.Machine = b.Machine
		r.Hour = r.Hour.UTC().Truncate(time.Hour)
		r.Input, r.Output = nonNeg(r.Input), nonNeg(r.Output)
		r.CacheRead, r.CacheWrite = nonNeg(r.CacheRead), nonNeg(r.CacheWrite)
		r.Reasoning, r.Sessions = nonNeg(r.Reasoning), nonNeg(r.Sessions)
		out = append(out, r)
	}
	return out
}

// negToNil turns an impossible negative measurement into "unknown".
func negToNil(p *float64) *float64 {
	if p != nil && *p < 0 {
		return nil
	}
	return p
}

// cleanNet stamps the batch machine and drops rows with an implausible timestamp or no target.
func cleanNet(b model.NetBatch, now time.Time) ([]model.NetSample, []model.SpeedTest) {
	samples := make([]model.NetSample, 0, len(b.Samples))
	for _, n := range b.Samples {
		if n.Target == "" || !timeOK(n.TS, now) {
			continue
		}
		n.Machine = b.Machine
		n.RTTMs, n.JitterMs, n.LossPct = negToNil(n.RTTMs), negToNil(n.JitterMs), negToNil(n.LossPct)
		samples = append(samples, n)
	}
	speed := make([]model.SpeedTest, 0, len(b.Speed))
	for _, sp := range b.Speed {
		if !timeOK(sp.TS, now) {
			continue
		}
		sp.Machine = b.Machine
		sp.DownMbps, sp.UpMbps = negToNil(sp.DownMbps), negToNil(sp.UpMbps)
		speed = append(speed, sp)
	}
	return samples, speed
}

// handleIngestUsage stores an agent's hourly usage rows. Re-posting a batch is harmless:
// the store keeps the larger value per column.
func (s *Server) handleIngestUsage(w http.ResponseWriter, r *http.Request) {
	var b model.UsageBatch
	if !decodeBatch(w, r, &b, func() string { return b.Machine }) {
		return
	}
	if err := s.Store.UpsertUsage(r.Context(), cleanUsage(b, time.Now())); err != nil {
		s.logf("ingest usage from %s: %v", b.Machine, err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "store failed"})
		return
	}
	writeJSON(w, http.StatusOK, model.IngestReply{})
}

// handleIngestNet stores latency samples and speed tests, and tells the agent whether
// someone asked for a speed test now.
func (s *Server) handleIngestNet(w http.ResponseWriter, r *http.Request) {
	var b model.NetBatch
	if !decodeBatch(w, r, &b, func() string { return b.Machine }) {
		return
	}
	samples, speed := cleanNet(b, time.Now())
	if err := s.Store.InsertNet(r.Context(), samples, speed); err != nil {
		s.logf("ingest net from %s: %v", b.Machine, err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "store failed"})
		return
	}
	writeJSON(w, http.StatusOK, model.IngestReply{RunSpeedtest: s.speed.take(b.Machine)})
}

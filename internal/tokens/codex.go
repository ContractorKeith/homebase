package tokens

import (
	"time"

	"github.com/ContractorKeith/homebase/internal/model"
)

type codexTotal struct {
	Input      int64 `json:"input_tokens"`
	Cached     int64 `json:"cached_input_tokens"`
	CacheWrite int64 `json:"cache_write_input_tokens"`
	Output     int64 `json:"output_tokens"`
	Reasoning  int64 `json:"reasoning_output_tokens"`
}

type codexLine struct {
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Payload   struct {
		Type  string `json:"type"`
		Model string `json:"model"`
		Info  *struct {
			Total *codexTotal `json:"total_token_usage"`
		} `json:"info"`
	} `json:"payload"`
}

// ParseCodex reads one Codex rollout. total_token_usage is cumulative for the session, so
// each event contributes its growth over the previous total; repeated totals add nothing.
// Input excludes the cached part, which Codex includes in input_tokens. Reasoning is part
// of Output already and is also stored separately.
func ParseCodex(path string) ([]model.Usage, error) {
	a := acc{}
	var prev codexTotal
	mdl := "unknown"
	err := eachLine(path, func(l codexLine) {
		p := l.Payload
		switch {
		case (l.Type == "turn_context" || l.Type == "session_meta") && p.Model != "":
			mdl = p.Model
		case l.Type == "event_msg" && p.Type == "token_count" && p.Info != nil && p.Info.Total != nil:
			t := *p.Info.Total
			if t.Input <= prev.Input && t.Output <= prev.Output && t.Cached <= prev.Cached &&
				t.CacheWrite <= prev.CacheWrite && t.Reasoning <= prev.Reasoning {
				return // repeated or non-growing total
			}
			if l.Timestamp.IsZero() {
				return
			}
			if t.Input < prev.Input || t.Output < prev.Output || t.Cached < prev.Cached ||
				t.CacheWrite < prev.CacheWrite || t.Reasoning < prev.Reasoning {
				prev = codexTotal{} // the counter restarted; never emit a negative delta
			}
			u := a.add(model.AgentCodex, mdl, l.Timestamp)
			u.Input += (t.Input - t.Cached) - (prev.Input - prev.Cached)
			u.CacheRead += t.Cached - prev.Cached
			u.CacheWrite += t.CacheWrite - prev.CacheWrite
			u.Output += t.Output - prev.Output
			u.Reasoning += t.Reasoning - prev.Reasoning
			prev = t
		}
	})
	return a.rows(), err
}

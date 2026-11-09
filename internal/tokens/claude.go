package tokens

import (
	"time"

	"github.com/ContractorKeith/homebase/internal/model"
)

type claudeLine struct {
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Message   struct {
		ID    string `json:"id"`
		Model string `json:"model"`
		Usage struct {
			Input      int64 `json:"input_tokens"`
			Output     int64 `json:"output_tokens"`
			CacheRead  int64 `json:"cache_read_input_tokens"`
			CacheWrite int64 `json:"cache_creation_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

// ParseClaude reads one Claude Code transcript. The same message.id repeats on several
// lines (one per content block); the first occurrence is counted once.
func ParseClaude(path string) ([]model.Usage, error) {
	a := acc{}
	seen := map[string]bool{}
	err := eachLine(path, func(l claudeLine) {
		m := l.Message
		if l.Type != "assistant" || m.Model == "" || m.Model == "<synthetic>" || l.Timestamp.IsZero() {
			return
		}
		if m.ID != "" {
			if seen[m.ID] {
				return
			}
			seen[m.ID] = true
		}
		u := a.add(model.AgentClaude, m.Model, l.Timestamp)
		u.Input += m.Usage.Input
		u.Output += m.Usage.Output
		u.CacheRead += m.Usage.CacheRead
		u.CacheWrite += m.Usage.CacheWrite
	})
	return a.rows(), err
}

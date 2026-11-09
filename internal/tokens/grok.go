package tokens

// Grok record choice.
//
// A Grok session directory holds events.jsonl, updates.jsonl, chat_history.jsonl and
// summary.json. Token numbers appear in two places in updates.jsonl: `_meta.totalTokens`
// on every streamed chunk (a running context size, repeated many times per turn) and the
// `usage` object of the single `turn_completed` update that ends each turn. events.jsonl,
// chat_history.jsonl and summary.json carry no usage.
//
// We count only `turn_completed.usage`: exactly one record per turn, per-turn rather than
// cumulative (successive turns do not grow by a whole context), and `usage.modelUsage`
// splits the turn by model, so a turn that switched models is attributed correctly. When
// modelUsage is absent we use the top-level counters with summary.json's current_model_id.
// inputTokens includes cachedReadTokens (totalTokens = input + output), so Input = input -
// cached read; reasoningTokens is part of outputTokens, so Output is left as reported.
// The record's unix-second `timestamp` picks the hour.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/ContractorKeith/homebase/internal/model"
)

type grokCounts struct {
	Input      int64 `json:"inputTokens"`
	Output     int64 `json:"outputTokens"`
	CacheRead  int64 `json:"cachedReadTokens"`
	CacheWrite int64 `json:"cacheCreationTokens"`
	Reasoning  int64 `json:"reasoningTokens"`
}

type grokLine struct {
	Timestamp int64 `json:"timestamp"`
	Params    struct {
		Update struct {
			Kind  string `json:"sessionUpdate"`
			Usage *struct {
				grokCounts
				ModelUsage map[string]grokCounts `json:"modelUsage"`
			} `json:"usage"`
		} `json:"update"`
	} `json:"params"`
}

// ParseGrok reads one Grok session's updates.jsonl.
func ParseGrok(path string) ([]model.Usage, error) {
	a := acc{}
	fallback := ""
	err := eachLine(path, func(l grokLine) {
		u := l.Params.Update
		if u.Kind != "turn_completed" || u.Usage == nil || l.Timestamp == 0 {
			return
		}
		per := u.Usage.ModelUsage
		if len(per) == 0 {
			if fallback == "" {
				fallback = grokSessionModel(filepath.Dir(path))
			}
			per = map[string]grokCounts{fallback: u.Usage.grokCounts}
		}
		for mdl, c := range per {
			r := a.add(model.AgentGrok, mdl, time.Unix(l.Timestamp, 0))
			r.Input += c.Input - c.CacheRead
			r.Output += c.Output
			r.CacheRead += c.CacheRead
			r.CacheWrite += c.CacheWrite
			r.Reasoning += c.Reasoning
		}
	})
	return a.rows(), err
}

// grokSessionModel reads current_model_id from the session's summary.json.
func grokSessionModel(dir string) string {
	b, err := os.ReadFile(filepath.Join(dir, "summary.json"))
	if err != nil {
		return "unknown"
	}
	var s struct {
		Model string `json:"current_model_id"`
	}
	if json.Unmarshal(b, &s) != nil || s.Model == "" {
		return "unknown"
	}
	return s.Model
}

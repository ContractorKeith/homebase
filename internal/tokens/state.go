package tokens

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/ContractorKeith/homebase/internal/model"
)

// fileEntry is one session file's remembered contribution.
type fileEntry struct {
	Agent string        `json:"agent"`
	Size  int64         `json:"size"`
	MTime int64         `json:"mtime"` // UnixNano
	Rows  []model.Usage `json:"rows"`
}

// state keeps each file's contributions by path. Entries are never dropped, so a deleted
// transcript still counts.
type state struct {
	Files map[string]*fileEntry `json:"files"`
}

func loadState(path string) (*state, error) {
	st := &state{Files: map[string]*fileEntry{}}
	if path == "" {
		return st, nil
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return st, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, st); err != nil {
		return nil, err
	}
	if st.Files == nil {
		st.Files = map[string]*fileEntry{}
	}
	return st, nil
}

// put stores a file's fresh rows. A re-parse never lowers what was stored for a bucket.
func (s *state) put(path, agent string, info fs.FileInfo, rows []model.Usage) {
	e := &fileEntry{Agent: agent, Size: info.Size(), MTime: info.ModTime().UnixNano(), Rows: rows}
	if old, ok := s.Files[path]; ok {
		e.Rows = mergeMax(old.Rows, rows)
	}
	s.Files[path] = e
}

func mergeMax(a, b []model.Usage) []model.Usage {
	m := acc{}
	for _, list := range [][]model.Usage{a, b} {
		for _, r := range list {
			u := m.add(r.Agent, r.Model, r.Hour)
			u.Input, u.Output = max(u.Input, r.Input), max(u.Output, r.Output)
			u.CacheRead, u.CacheWrite = max(u.CacheRead, r.CacheRead), max(u.CacheWrite, r.CacheWrite)
			u.Reasoning = max(u.Reasoning, r.Reasoning)
		}
	}
	return m.rows()
}

// totals sums every file's rows into (agent, model, hour) buckets for the wanted agents.
func (s *state) totals(want map[string]bool, machine string) []model.Usage {
	sum := map[key]*model.Usage{}
	for _, e := range s.Files {
		if len(want) > 0 && !want[e.Agent] {
			continue
		}
		for _, r := range e.Rows {
			k := key{r.Agent, r.Model, r.Hour.UTC().Truncate(time.Hour)}
			u, ok := sum[k]
			if !ok {
				u = &model.Usage{Machine: machine, Agent: r.Agent, Model: r.Model, Hour: k.Hour}
				sum[k] = u
			}
			u.Input += r.Input
			u.Output += r.Output
			u.CacheRead += r.CacheRead
			u.CacheWrite += r.CacheWrite
			u.Reasoning += r.Reasoning
			u.Sessions += r.Sessions
		}
	}
	out := make([]model.Usage, 0, len(sum))
	for _, u := range sum {
		out = append(out, *u)
	}
	sortRows(out)
	return out
}

// save writes the state with 0600 permissions, atomically.
func (s *state) save(path string) error {
	if path == "" {
		return nil
	}
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Package tokens parses local agent session files into hourly model.Usage rows.
// Parsers only read; they never write session files.
package tokens

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"sort"
	"time"

	"github.com/ContractorKeith/homebase/internal/model"
)

// key identifies one (agent, model, hour) bucket.
type key struct {
	Agent, Model string
	Hour         time.Time
}

// acc sums contributions from one file into hourly buckets.
type acc map[key]*model.Usage

// add returns the bucket for k, creating it with Sessions = 1 (one file per bucket).
func (a acc) add(agent, mdl string, t time.Time) *model.Usage {
	k := key{agent, mdl, t.UTC().Truncate(time.Hour)}
	u, ok := a[k]
	if !ok {
		u = &model.Usage{Agent: agent, Model: mdl, Hour: k.Hour, Sessions: 1}
		a[k] = u
	}
	return u
}

// rows returns the buckets sorted by hour, agent, model.
func (a acc) rows() []model.Usage {
	out := make([]model.Usage, 0, len(a))
	for _, u := range a {
		out = append(out, *u)
	}
	sortRows(out)
	return out
}

func sortRows(r []model.Usage) {
	sort.Slice(r, func(i, j int) bool {
		if !r[i].Hour.Equal(r[j].Hour) {
			return r[i].Hour.Before(r[j].Hour)
		}
		if r[i].Agent != r[j].Agent {
			return r[i].Agent < r[j].Agent
		}
		return r[i].Model < r[j].Model
	})
}

// eachLine decodes every JSON line of a file into v (fresh per line) and calls fn.
// Lines that are not valid JSON are skipped.
func eachLine[T any](path string, fn func(T)) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, 1<<20)
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 {
			var v T
			if json.Unmarshal(line, &v) == nil {
				fn(v)
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

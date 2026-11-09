package hub

import (
	"net/http"
	"sort"
	"time"

	"github.com/ContractorKeith/homebase/internal/model"
	"github.com/ContractorKeith/homebase/internal/ranges"
)

// viewRange is the parsed range and zone of a range API request.
type viewRange struct {
	name string
	loc  *time.Location
	now  time.Time
}

// parseView reads range and tz; it writes the 400 itself and reports false on a bad value.
func (s *Server) parseView(w http.ResponseWriter, r *http.Request) (viewRange, bool) {
	q := r.URL.Query()
	v := viewRange{name: q.Get("range"), loc: time.Local, now: s.Now()}
	if !ranges.Valid(v.name) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "range must be today, 7d, 30d, 90d or ttm"})
		return v, false
	}
	if tz := q.Get("tz"); tz != "" {
		loc, err := time.LoadLocation(tz)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "tz must be an IANA zone such as America/New_York"})
			return v, false
		}
		v.loc = loc
	}
	if s.Store == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "no database"})
		return v, false
	}
	return v, true
}

// bucketJSON is one axis bucket: start is RFC 3339 in the viewer's zone.
type bucketJSON struct {
	Start string `json:"start"`
	Label string `json:"label"`
}

func bucketsJSON(bs []ranges.Bucket) []bucketJSON {
	out := make([]bucketJSON, len(bs))
	for i, b := range bs {
		out[i] = bucketJSON{b.Start.Format(time.RFC3339), b.Label}
	}
	return out
}

// tokenCounts is a sum over a range; Total = input + output + cacheRead + cacheWrite.
// Reasoning is shown but not added: it is already inside output.
type tokenCounts struct {
	Total      int64 `json:"total"`
	Input      int64 `json:"input"`
	Output     int64 `json:"output"`
	CacheRead  int64 `json:"cacheRead"`
	CacheWrite int64 `json:"cacheWrite"`
	Reasoning  int64 `json:"reasoning"`
}

func (c *tokenCounts) add(u model.Usage) {
	c.Input += u.Input
	c.Output += u.Output
	c.CacheRead += u.CacheRead
	c.CacheWrite += u.CacheWrite
	c.Reasoning += u.Reasoning
	c.Total += u.Input + u.Output + u.CacheRead + u.CacheWrite
}

// tokenSeries holds one group's value per bucket for each token type.
type tokenSeries struct {
	Key        string  `json:"key"`
	Name       string  `json:"name"`
	Slot       int     `json:"slot"` // 1-8 stable color slot; 0 = no slot, draw as "other"
	Total      []int64 `json:"total"`
	Input      []int64 `json:"input"`
	Output     []int64 `json:"output"`
	CacheRead  []int64 `json:"cacheRead"`
	CacheWrite []int64 `json:"cacheWrite"`
	Reasoning  []int64 `json:"reasoning"`
	sum        int64
}

const colorSlots = 8

// groupKey names the group a row belongs to for the by= choice.
func groupKey(by string, u model.Usage) string {
	switch by {
	case "model":
		if u.Model == "" {
			return "unknown"
		}
		return u.Model
	case "machine":
		return u.Machine
	}
	return u.Agent
}

func newTokenSeries(key, name string, slot, n int) *tokenSeries {
	mk := func() []int64 { return make([]int64, n) }
	return &tokenSeries{Key: key, Name: name, Slot: slot, Total: mk(), Input: mk(), Output: mk(),
		CacheRead: mk(), CacheWrite: mk(), Reasoning: mk()}
}

func (t *tokenSeries) add(i int, u model.Usage) {
	t.Input[i] += u.Input
	t.Output[i] += u.Output
	t.CacheRead[i] += u.CacheRead
	t.CacheWrite[i] += u.CacheWrite
	t.Reasoning[i] += u.Reasoning
	v := u.Input + u.Output + u.CacheRead + u.CacheWrite
	t.Total[i] += v
	t.sum += v
}

// slots gives each group a color slot by when it first appeared in the whole history, so a
// group keeps its color in every range and new groups never repaint old ones. rows are
// ordered by hour.
func slots(by string, rows []model.Usage) map[string]int {
	out := map[string]int{}
	if by == "agent" { // the three agents always wear the same colors
		out[model.AgentClaude], out[model.AgentCodex], out[model.AgentGrok] = 1, 2, 3
	}
	for _, u := range rows {
		k := groupKey(by, u)
		if _, ok := out[k]; !ok {
			n := len(out) + 1
			if n > colorSlots {
				n = 0
			}
			out[k] = n
		}
	}
	return out
}

// machineNames maps machine ids to card names when machine-monitor has reported them.
func (s *Server) machineNames() map[string]string {
	out := map[string]string{}
	if list, ok := s.feed.current(); ok {
		for _, m := range list {
			out[m.ID] = m.Name
		}
	}
	return out
}

// handleTokens serves GET /api/tokens?range=&by=agent|model|machine&tz=.
func (s *Server) handleTokens(w http.ResponseWriter, r *http.Request) {
	v, ok := s.parseView(w, r)
	if !ok {
		return
	}
	by := r.URL.Query().Get("by")
	if by == "" {
		by = "agent"
	}
	if by != "agent" && by != "model" && by != "machine" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "by must be agent, model or machine"})
		return
	}
	cur, prev, _ := ranges.Buckets(v.name, v.now, v.loc)
	end := cur[len(cur)-1].End
	rows, err := s.Store.UsageBetween(r.Context(), time.Unix(0, 0), end)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	slot, names := slots(by, rows), s.machineNames()
	byKey := map[string]*tokenSeries{}
	var totals, prevTotals tokenCounts
	for _, u := range rows {
		if i := ranges.Find(cur, u.Hour); i >= 0 {
			k := groupKey(by, u)
			ts := byKey[k]
			if ts == nil {
				name := k
				if by == "machine" && names[k] != "" {
					name = names[k]
				}
				ts = newTokenSeries(k, name, slot[k], len(cur))
				byKey[k] = ts
			}
			ts.add(i, u)
			totals.add(u)
		} else if !u.Hour.Before(prev[0].Start) && u.Hour.Before(prev[len(prev)-1].End) {
			prevTotals.add(u)
		}
	}
	series := make([]*tokenSeries, 0, len(byKey))
	for _, ts := range byKey {
		series = append(series, ts)
	}
	sort.Slice(series, func(i, j int) bool {
		if series[i].sum != series[j].sum {
			return series[i].sum > series[j].sum
		}
		return series[i].Key < series[j].Key
	})
	writeJSON(w, http.StatusOK, map[string]any{"range": v.name, "by": by, "tz": v.loc.String(),
		"anyData": len(rows) > 0, "buckets": bucketsJSON(cur), "series": series, "totals": totals, "prevTotals": prevTotals})
}

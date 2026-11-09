package hub

import (
	"net/http"
	"sort"
	"time"

	"github.com/ContractorKeith/homebase/internal/model"
	"github.com/ContractorKeith/homebase/internal/ranges"
)

// ghCounts is a sum of GitHub activity.
type ghCounts struct {
	Commits   int64 `json:"commits"`
	Additions int64 `json:"additions"`
	Deletions int64 `json:"deletions"`
	PRsOpened int64 `json:"prsOpened"`
	PRsMerged int64 `json:"prsMerged"`
	Issues    int64 `json:"issues"`
	Reviews   int64 `json:"reviews"`
}

func (c *ghCounts) add(d model.GitHubDay) {
	c.Commits += d.Commits
	c.Additions += d.Additions
	c.Deletions += d.Deletions
	c.PRsOpened += d.PRsOpened
	c.PRsMerged += d.PRsMerged
	c.Issues += d.Issues
	c.Reviews += d.Reviews
}

// ghSeries is each metric's value per bucket.
type ghSeries struct {
	Commits   []int64 `json:"commits"`
	Additions []int64 `json:"additions"`
	Deletions []int64 `json:"deletions"`
	PRsOpened []int64 `json:"prsOpened"`
	PRsMerged []int64 `json:"prsMerged"`
	Issues    []int64 `json:"issues"`
	Reviews   []int64 `json:"reviews"`
}

func newGHSeries(n int) ghSeries {
	mk := func() []int64 { return make([]int64, n) }
	return ghSeries{mk(), mk(), mk(), mk(), mk(), mk(), mk()}
}

func (g *ghSeries) add(i int, d model.GitHubDay) {
	g.Commits[i] += d.Commits
	g.Additions[i] += d.Additions
	g.Deletions[i] += d.Deletions
	g.PRsOpened[i] += d.PRsOpened
	g.PRsMerged[i] += d.PRsMerged
	g.Issues[i] += d.Issues
	g.Reviews[i] += d.Reviews
}

type repoTotals struct {
	Repo string `json:"repo"`
	ghCounts
}

const topRepos = 10

// handleGitHub serves GET /api/github?range=&tz=. Data is per day, so "today" is one bucket
// and weekly buckets sum days.
func (s *Server) handleGitHub(w http.ResponseWriter, r *http.Request) {
	v, ok := s.parseView(w, r)
	if !ok {
		return
	}
	cur, prev, _ := ranges.DayBuckets(v.name, v.now, v.loc)
	const dayFmt = "2006-01-02"
	rows, err := s.Store.GitHubBetween(r.Context(), prev[0].Start.Format(dayFmt), cur[len(cur)-1].End.Format(dayFmt))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	series := newGHSeries(len(cur))
	var totals, prevTotals ghCounts
	repos := map[string]*repoTotals{}
	for _, d := range rows {
		// A stored day is a calendar date; read it as that date in the viewer's zone.
		t, err := time.ParseInLocation(dayFmt, d.Day, v.loc)
		if err != nil {
			continue
		}
		if i := ranges.Find(cur, t); i >= 0 {
			series.add(i, d)
			totals.add(d)
			rt := repos[d.Repo]
			if rt == nil {
				rt = &repoTotals{Repo: d.Repo}
				repos[d.Repo] = rt
			}
			rt.add(d)
		} else if ranges.Find(prev, t) >= 0 {
			prevTotals.add(d)
		}
	}
	top := make([]*repoTotals, 0, len(repos))
	for _, rt := range repos {
		top = append(top, rt)
	}
	sort.Slice(top, func(i, j int) bool {
		if top[i].Commits != top[j].Commits {
			return top[i].Commits > top[j].Commits
		}
		return top[i].Repo < top[j].Repo
	})
	if len(top) > topRepos {
		top = top[:topRepos]
	}
	writeJSON(w, http.StatusOK, map[string]any{"range": v.name, "tz": v.loc.String(),
		"buckets": bucketsJSON(cur), "series": series, "totals": totals, "prevTotals": prevTotals, "repos": top})
}

package hub

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/ContractorKeith/homebase/internal/model"
	"github.com/ContractorKeith/homebase/internal/store"
)

type fakeSource struct {
	spans []time.Duration
	fail  bool
}

func (f *fakeSource) Fetch(_ context.Context, from, to time.Time) ([]model.GitHubDay, error) {
	if f.fail {
		return nil, errors.New("gh down")
	}
	f.spans = append(f.spans, to.Sub(from))
	return []model.GitHubDay{{Repo: "o/r", Day: "2026-11-07", Commits: 2}}, nil
}

func TestGitHubSyncBackfillsOnceThenRefreshes(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "h.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	src := &fakeSource{fail: true}
	g := &GitHubSync{Store: st, Src: src, Logf: t.Logf}
	ctx := context.Background()
	if g.Pass(ctx) == nil {
		t.Fatal("want the fetch error")
	}
	if _, done, _ := st.GetMeta(ctx, backfilledKey); done {
		t.Fatal("a failed backfill must not be recorded")
	}
	src.fail = false
	for i := 0; i < 2; i++ {
		if err := g.Pass(ctx); err != nil {
			t.Fatal(err)
		}
	}
	day := 24 * time.Hour
	if len(src.spans) != 2 || src.spans[0] != backfillDays*day || src.spans[1] != refreshDays*day {
		t.Fatalf("spans %v", src.spans)
	}
	rows, _ := st.GitHubBetween(ctx, "2026-01-01", "2027-01-01")
	if len(rows) != 1 || rows[0].Commits != 2 {
		t.Fatalf("rows %+v", rows)
	}
}

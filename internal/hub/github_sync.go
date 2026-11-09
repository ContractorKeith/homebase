package hub

import (
	"context"
	"time"

	"github.com/ContractorKeith/homebase/internal/model"
	"github.com/ContractorKeith/homebase/internal/store"
)

const (
	backfilledKey  = "github_backfilled"
	backfillDays   = 365
	refreshDays    = 3
	githubEvery    = 30 * time.Minute
	githubRetryMin = 5 * time.Minute
)

// GitHubSource fetches Keith's activity; *github.Client satisfies it.
type GitHubSource interface {
	Fetch(ctx context.Context, from, to time.Time) ([]model.GitHubDay, error)
}

// GitHubSync keeps the store's GitHub days current. The first run backfills a year;
// after that it refetches the last few days every 30 minutes.
type GitHubSync struct {
	Store *store.Store
	Src   GitHubSource
	Logf  func(format string, args ...any)
	Every time.Duration // zero means 30 minutes
	Retry time.Duration // wait after a failed pass; zero means 5 minutes
	Now   func() time.Time
}

// Pass runs one fetch and stores it.
func (g *GitHubSync) Pass(ctx context.Context) error {
	now := time.Now
	if g.Now != nil {
		now = g.Now
	}
	_, done, err := g.Store.GetMeta(ctx, backfilledKey)
	if err != nil {
		return err
	}
	days := refreshDays
	if !done {
		days = backfillDays
	}
	t := now()
	rows, err := g.Src.Fetch(ctx, t.AddDate(0, 0, -days), t)
	if err != nil {
		return err
	}
	if err := g.Store.ReplaceGitHubDays(ctx, rows); err != nil {
		return err
	}
	if !done {
		return g.Store.SetMeta(ctx, backfilledKey, t.UTC().Format(time.RFC3339))
	}
	return nil
}

// Run passes until ctx ends. A failed pass is logged and retried sooner than the normal cycle.
func (g *GitHubSync) Run(ctx context.Context) {
	every, retry := g.Every, g.Retry
	if every <= 0 {
		every = githubEvery
	}
	if retry <= 0 {
		retry = githubRetryMin
	}
	for {
		wait := every
		if err := g.Pass(ctx); err != nil && ctx.Err() == nil {
			g.Logf("github: %v (retrying in %s)", err, retry)
			wait = retry
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/ContractorKeith/homebase/internal/model"
)

// UpsertUsage stores hourly totals. Each column keeps the larger of the stored and the
// incoming value, so a re-scan after an agent deletes old transcripts never lowers history
// and sending the same batch twice changes nothing.
func (s *Store) UpsertUsage(ctx context.Context, rows []model.Usage) error {
	return s.tx(ctx, func(t *sql.Tx) error {
		st, err := t.PrepareContext(ctx, `INSERT INTO usage
			(machine, agent, model, hour, input, output, cache_read, cache_write, reasoning, sessions)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (machine, agent, model, hour) DO UPDATE SET
				input = max(input, excluded.input), output = max(output, excluded.output),
				cache_read = max(cache_read, excluded.cache_read),
				cache_write = max(cache_write, excluded.cache_write),
				reasoning = max(reasoning, excluded.reasoning), sessions = max(sessions, excluded.sessions)`)
		if err != nil {
			return err
		}
		defer st.Close()
		for _, r := range rows {
			if r.Machine == "" || r.Agent == "" {
				return fmt.Errorf("store: usage row needs machine and agent")
			}
			h := r.Hour.UTC().Truncate(time.Hour)
			if _, err := st.ExecContext(ctx, r.Machine, r.Agent, r.Model, unix(h), r.Input, r.Output,
				r.CacheRead, r.CacheWrite, r.Reasoning, r.Sessions); err != nil {
				return err
			}
		}
		return nil
	})
}

// UsageBetween returns usage rows with from <= hour < to, ordered by hour.
func (s *Store) UsageBetween(ctx context.Context, from, to time.Time) ([]model.Usage, error) {
	rs, err := s.db.QueryContext(ctx, `SELECT machine, agent, model, hour, input, output, cache_read,
		cache_write, reasoning, sessions FROM usage WHERE hour >= ? AND hour < ?
		ORDER BY hour, machine, agent, model`, unix(from), unix(to))
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var out []model.Usage
	for rs.Next() {
		var u model.Usage
		var h int64
		if err := rs.Scan(&u.Machine, &u.Agent, &u.Model, &h, &u.Input, &u.Output, &u.CacheRead,
			&u.CacheWrite, &u.Reasoning, &u.Sessions); err != nil {
			return nil, err
		}
		u.Hour = fromUnix(h)
		out = append(out, u)
	}
	return out, rs.Err()
}

// InsertNet stores latency samples and speed tests; duplicates (same key) are ignored.
func (s *Store) InsertNet(ctx context.Context, samples []model.NetSample, speed []model.SpeedTest) error {
	return s.tx(ctx, func(t *sql.Tx) error {
		for _, n := range samples {
			if _, err := t.ExecContext(ctx, `INSERT OR IGNORE INTO net_samples
				(machine, target, ts, rtt_ms, jitter_ms, loss_pct) VALUES (?, ?, ?, ?, ?, ?)`,
				n.Machine, n.Target, unix(n.TS), n.RTTMs, n.JitterMs, n.LossPct); err != nil {
				return err
			}
		}
		for _, sp := range speed {
			if _, err := t.ExecContext(ctx, `INSERT OR IGNORE INTO speed_tests
				(machine, ts, down_mbps, up_mbps, server) VALUES (?, ?, ?, ?, ?)`,
				sp.Machine, unix(sp.TS), sp.DownMbps, sp.UpMbps, sp.Server); err != nil {
				return err
			}
		}
		return nil
	})
}

// NetBetween returns latency samples with from <= ts < to, ordered by ts.
func (s *Store) NetBetween(ctx context.Context, from, to time.Time) ([]model.NetSample, error) {
	rs, err := s.db.QueryContext(ctx, `SELECT machine, target, ts, rtt_ms, jitter_ms, loss_pct
		FROM net_samples WHERE ts >= ? AND ts < ? ORDER BY ts, machine, target`, unix(from), unix(to))
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var out []model.NetSample
	for rs.Next() {
		var n model.NetSample
		var ts int64
		if err := rs.Scan(&n.Machine, &n.Target, &ts, &n.RTTMs, &n.JitterMs, &n.LossPct); err != nil {
			return nil, err
		}
		n.TS = fromUnix(ts)
		out = append(out, n)
	}
	return out, rs.Err()
}

// SpeedBetween returns speed tests with from <= ts < to, ordered by ts.
func (s *Store) SpeedBetween(ctx context.Context, from, to time.Time) ([]model.SpeedTest, error) {
	rs, err := s.db.QueryContext(ctx, `SELECT machine, ts, down_mbps, up_mbps, server
		FROM speed_tests WHERE ts >= ? AND ts < ? ORDER BY ts, machine`, unix(from), unix(to))
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var out []model.SpeedTest
	for rs.Next() {
		var sp model.SpeedTest
		var ts int64
		if err := rs.Scan(&sp.Machine, &ts, &sp.DownMbps, &sp.UpMbps, &sp.Server); err != nil {
			return nil, err
		}
		sp.TS = fromUnix(ts)
		out = append(out, sp)
	}
	return out, rs.Err()
}

// ReplaceGitHubDays stores GitHub days. GitHub is re-fetchable, so a fetched day
// replaces what was stored for it.
func (s *Store) ReplaceGitHubDays(ctx context.Context, days []model.GitHubDay) error {
	return s.tx(ctx, func(t *sql.Tx) error {
		for _, d := range days {
			if _, err := t.ExecContext(ctx, `INSERT OR REPLACE INTO github_days
				(repo, day, commits, additions, deletions, prs_opened, prs_merged, issues, reviews)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, d.Repo, d.Day, d.Commits, d.Additions, d.Deletions,
				d.PRsOpened, d.PRsMerged, d.Issues, d.Reviews); err != nil {
				return err
			}
		}
		return nil
	})
}

// GitHubBetween returns GitHub days with from <= day < to (YYYY-MM-DD), ordered by day.
func (s *Store) GitHubBetween(ctx context.Context, from, to string) ([]model.GitHubDay, error) {
	rs, err := s.db.QueryContext(ctx, `SELECT repo, day, commits, additions, deletions, prs_opened,
		prs_merged, issues, reviews FROM github_days WHERE day >= ? AND day < ? ORDER BY day, repo`, from, to)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var out []model.GitHubDay
	for rs.Next() {
		var d model.GitHubDay
		if err := rs.Scan(&d.Repo, &d.Day, &d.Commits, &d.Additions, &d.Deletions, &d.PRsOpened,
			&d.PRsMerged, &d.Issues, &d.Reviews); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rs.Err()
}

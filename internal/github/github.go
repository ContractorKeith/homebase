// Package github fetches Keith's authored GitHub activity through the gh CLI: commits, PRs,
// issues and reviews from the contributions API, and lines changed from commit history.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/ContractorKeith/homebase/internal/model"
)

const window = 90 * 24 * time.Hour // keeps one repo's per-day commit list under 100 nodes

// Runner runs one GraphQL query with string variables and returns the raw JSON reply.
// Tests replace it with recorded replies.
type Runner func(ctx context.Context, query string, vars map[string]string) ([]byte, error)

// ExecRunner runs the query through `gh api graphql`.
func ExecRunner(ctx context.Context, query string, vars map[string]string) ([]byte, error) {
	args := []string{"api", "graphql", "-f", "query=" + query}
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		args = append(args, "-f", k+"="+vars[k])
	}
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("gh: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// Available reports whether the gh CLI is on PATH.
func Available() bool {
	_, err := exec.LookPath("gh")
	return err == nil
}

// Client fetches GitHub days. The zero value uses ExecRunner.
type Client struct {
	Run Runner
}

type pageInfo struct {
	HasNextPage bool
	EndCursor   string
}

type page[T any] struct {
	Nodes    []T
	PageInfo pageInfo
}

// query runs q and decodes the reply's data object into out.
func (c *Client) query(ctx context.Context, q string, vars map[string]string, out any) error {
	run := c.Run
	if run == nil {
		run = ExecRunner
	}
	b, err := run(ctx, q, vars)
	if err != nil {
		return err
	}
	var env struct {
		Data   json.RawMessage
		Errors []struct{ Message string }
	}
	if err := json.Unmarshal(b, &env); err != nil {
		return fmt.Errorf("github: bad reply: %w", err)
	}
	if len(env.Errors) > 0 {
		return errors.New("github: " + env.Errors[0].Message)
	}
	return json.Unmarshal(env.Data, out)
}

// pages follows a connection to its end. get picks the connection out of each reply.
func pages[R, T any](ctx context.Context, c *Client, q string, vars map[string]string, get func(*R) page[T]) ([]T, error) {
	v := map[string]string{}
	for k, s := range vars {
		v[k] = s
	}
	var all []T
	for {
		var r R
		if err := c.query(ctx, q, v, &r); err != nil {
			return nil, err
		}
		p := get(&r)
		all = append(all, p.Nodes...)
		if !p.PageInfo.HasNextPage || p.PageInfo.EndCursor == "" {
			return all, nil
		}
		v["after"] = p.PageInfo.EndCursor
	}
}

// days accumulates activity per (repo, UTC day).
type days map[[2]string]*model.GitHubDay

func (d days) at(repo string, t time.Time) *model.GitHubDay {
	k := [2]string{repo, t.UTC().Format("2006-01-02")}
	if d[k] == nil {
		d[k] = &model.GitHubDay{Repo: repo, Day: k[1]}
	}
	return d[k]
}

func (d days) sorted() []model.GitHubDay {
	out := make([]model.GitHubDay, 0, len(d))
	for _, v := range d {
		out = append(out, *v)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Day != out[j].Day {
			return out[i].Day < out[j].Day
		}
		return out[i].Repo < out[j].Repo
	})
	return out
}

// Fetch returns Keith's activity from the start of from's UTC day to to. Every returned
// day is complete, so the caller can replace stored days with it.
func (c *Client) Fetch(ctx context.Context, from, to time.Time) ([]model.GitHubDay, error) {
	from = from.UTC().Truncate(24 * time.Hour)
	to = to.UTC()
	var who struct{ Viewer struct{ ID, Login string } }
	if err := c.query(ctx, viewerQuery, nil, &who); err != nil {
		return nil, err
	}
	acc := days{}
	repos := map[string]bool{}
	for start := from; start.Before(to); start = start.Add(window) {
		end := start.Add(window)
		if end.After(to) {
			end = to
		}
		if err := c.counts(ctx, start, end, who.Viewer.Login, acc, repos); err != nil {
			return nil, err
		}
	}
	if err := c.lines(ctx, from, to, who.Viewer.ID, repos, acc); err != nil {
		return nil, err
	}
	return acc.sorted(), nil
}

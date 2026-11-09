package github

import (
	"context"
	"sort"
	"strings"
	"time"
)

const historyQuery = `query History($owner: String!, $name: String!, $id: ID!, $since: GitTimestamp!, $until: GitTimestamp!, $after: String) {
  repository(owner: $owner, name: $name) {
    defaultBranchRef { target { ... on Commit {
      history(author: {id: $id}, since: $since, until: $until, first: 100, after: $after) {
        nodes { committedDate additions deletions }
        pageInfo { hasNextPage endCursor }
      }
    } } }
  }
}`

type commitNode struct {
	CommittedDate time.Time
	Additions     int64
	Deletions     int64
}

type history struct {
	Repository struct {
		DefaultBranchRef *struct {
			Target struct {
				History page[commitNode]
			}
		}
	}
}

// lines adds additions and deletions from Keith's commits on each repo's default branch.
// A repo whose history is empty (a squash merge authored by GitHub, say) adds nothing.
func (c *Client) lines(ctx context.Context, from, to time.Time, userID string, repos map[string]bool, acc days) error {
	names := make([]string, 0, len(repos))
	for r := range repos {
		names = append(names, r)
	}
	sort.Strings(names)
	for _, repo := range names {
		owner, name, ok := strings.Cut(repo, "/")
		if !ok {
			continue
		}
		v := map[string]string{"owner": owner, "name": name, "id": userID,
			"since": from.Format(time.RFC3339), "until": to.Format(time.RFC3339)}
		commits, err := pages(ctx, c, historyQuery, v, func(r *history) page[commitNode] {
			if r.Repository.DefaultBranchRef == nil {
				return page[commitNode]{}
			}
			return r.Repository.DefaultBranchRef.Target.History
		})
		if err != nil {
			return err
		}
		for _, n := range commits {
			d := acc.at(repo, n.CommittedDate)
			d.Additions += n.Additions
			d.Deletions += n.Deletions
		}
	}
	return nil
}

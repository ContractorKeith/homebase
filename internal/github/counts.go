package github

import (
	"context"
	"time"
)

const viewerQuery = `query Viewer { viewer { id login } }`

const commitsQuery = `query Commits($from: DateTime!, $to: DateTime!) {
  viewer { contributionsCollection(from: $from, to: $to) {
    commitContributionsByRepository(maxRepositories: 100) {
      repository { nameWithOwner }
      contributions(first: 100) { nodes { occurredAt commitCount } }
    }
  } }
}`

const prsQuery = `query PRs($from: DateTime!, $to: DateTime!, $after: String) {
  viewer { contributionsCollection(from: $from, to: $to) {
    pullRequestContributions(first: 100, after: $after) {
      nodes { pullRequest { createdAt repository { nameWithOwner } } }
      pageInfo { hasNextPage endCursor }
    }
  } }
}`

const issuesQuery = `query Issues($from: DateTime!, $to: DateTime!, $after: String) {
  viewer { contributionsCollection(from: $from, to: $to) {
    issueContributions(first: 100, after: $after) {
      nodes { issue { createdAt repository { nameWithOwner } } }
      pageInfo { hasNextPage endCursor }
    }
  } }
}`

const reviewsQuery = `query Reviews($from: DateTime!, $to: DateTime!, $after: String) {
  viewer { contributionsCollection(from: $from, to: $to) {
    pullRequestReviewContributions(first: 100, after: $after) {
      nodes { occurredAt repository { nameWithOwner } }
      pageInfo { hasNextPage endCursor }
    }
  } }
}`

// Merges come from search, not contributions, so a PR opened before the window and merged
// inside it still counts on its merge day.
const mergedQuery = `query Merged($q: String!, $after: String) {
  search(query: $q, type: ISSUE, first: 100, after: $after) {
    nodes { ... on PullRequest { mergedAt repository { nameWithOwner } } }
    pageInfo { hasNextPage endCursor }
  }
}`

type repoName struct{ NameWithOwner string }

type prNode struct {
	PullRequest struct {
		CreatedAt  time.Time
		Repository repoName
	}
}

type issueNode struct {
	Issue struct {
		CreatedAt  time.Time
		Repository repoName
	}
}

type reviewNode struct {
	OccurredAt time.Time
	Repository repoName
}

type mergedNode struct {
	MergedAt   *time.Time
	Repository repoName
}

type collection struct {
	Viewer struct {
		ContributionsCollection struct {
			CommitContributionsByRepository []struct {
				Repository    repoName
				Contributions struct {
					Nodes []struct {
						OccurredAt  time.Time
						CommitCount int64
					}
				}
			}
			PullRequestContributions       page[prNode]
			IssueContributions             page[issueNode]
			PullRequestReviewContributions page[reviewNode]
		}
	}
}

type mergedSearch struct {
	Search page[mergedNode]
}

// counts adds commits, PRs, issues, reviews and merges for [from, to) to acc, and records
// the repos with commits in repos.
func (c *Client) counts(ctx context.Context, from, to time.Time, login string, acc days, repos map[string]bool) error {
	v := map[string]string{"from": from.Format(time.RFC3339), "to": to.Format(time.RFC3339)}

	var cm collection
	if err := c.query(ctx, commitsQuery, v, &cm); err != nil {
		return err
	}
	for _, r := range cm.Viewer.ContributionsCollection.CommitContributionsByRepository {
		repos[r.Repository.NameWithOwner] = true
		for _, n := range r.Contributions.Nodes {
			acc.at(r.Repository.NameWithOwner, n.OccurredAt).Commits += n.CommitCount
		}
	}

	prs, err := pages(ctx, c, prsQuery, v, func(r *collection) page[prNode] {
		return r.Viewer.ContributionsCollection.PullRequestContributions
	})
	if err != nil {
		return err
	}
	for _, n := range prs {
		acc.at(n.PullRequest.Repository.NameWithOwner, n.PullRequest.CreatedAt).PRsOpened++
	}

	issues, err := pages(ctx, c, issuesQuery, v, func(r *collection) page[issueNode] {
		return r.Viewer.ContributionsCollection.IssueContributions
	})
	if err != nil {
		return err
	}
	for _, n := range issues {
		acc.at(n.Issue.Repository.NameWithOwner, n.Issue.CreatedAt).Issues++
	}

	reviews, err := pages(ctx, c, reviewsQuery, v, func(r *collection) page[reviewNode] {
		return r.Viewer.ContributionsCollection.PullRequestReviewContributions
	})
	if err != nil {
		return err
	}
	for _, n := range reviews {
		acc.at(n.Repository.NameWithOwner, n.OccurredAt).Reviews++
	}

	// The search range is inclusive at both ends, so stop one second short of the next window.
	q := "author:" + login + " is:pr is:merged merged:" + from.Format(time.RFC3339) + ".." + to.Add(-time.Second).Format(time.RFC3339)
	merged, err := pages(ctx, c, mergedQuery, map[string]string{"q": q}, func(r *mergedSearch) page[mergedNode] {
		return r.Search
	})
	if err != nil {
		return err
	}
	for _, n := range merged {
		if n.MergedAt != nil {
			acc.at(n.Repository.NameWithOwner, *n.MergedAt).PRsMerged++
		}
	}
	return nil
}

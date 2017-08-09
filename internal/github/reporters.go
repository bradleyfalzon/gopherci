package github

import (
	"context"

	"github.com/bradleyfalzon/gopherci/internal/analyser"
	"github.com/bradleyfalzon/gopherci/internal/db"
	"github.com/google/go-github/github"
	"github.com/pkg/errors"
)

// PRCommentReporter is a analyser.Reporter that creates a pull request comment
// for each issue on a given owner, repo, pr and commit hash. Returns on the
// first error encountered.
type PRCommentReporter struct {
	client *github.Client
	owner  string
	repo   string
	number int
	commit string
}

var _ analyser.Reporter = &PRCommentReporter{}

// NewPRCommentReporter returns a PRCommentReporter.
func NewPRCommentReporter(client *github.Client, owner, repo string, number int, commit string) *PRCommentReporter {
	return &PRCommentReporter{
		client: client,
		owner:  owner,
		repo:   repo,
		number: number,
		commit: commit,
	}
}

// FilterIssues deduplicates issues by checking the existing pull request for
// existing comments and returns comments that don't already exist.
func (r *PRCommentReporter) filterIssues(ctx context.Context, issues []db.Issue) (filtered []db.Issue, err error) {
	ecomments, _, err := r.client.PullRequests.ListComments(ctx, r.owner, r.repo, r.number, nil)
	if err != nil {
		return nil, errors.Wrap(err, "could not list existing comments")
	}

	// remove duplicate comments, as we're remove elements based on the index
	// start from last position and work backwards to keep indexes consistent
	// even after removing elements.
	for i := len(issues) - 1; i >= 0; i-- {
		issue := issues[i]
		for _, ec := range ecomments {
			if ec.Path == nil || ec.Position == nil || ec.Body == nil {
				continue
			}
			if issue.Path == *ec.Path && issue.HunkPos == *ec.Position && issue.Issue == *ec.Body {
				issues = append(issues[:i], issues[i+1:]...)
				break
			}
		}
	}

	return issues, nil
}

// Report implements the analyser.Reporter interface.
func (r *PRCommentReporter) Report(ctx context.Context, issues []db.Issue) error {
	filtered, err := r.filterIssues(ctx, issues)
	if err != nil {
		return err
	}

	_, issues = analyser.Suppress(filtered, analyser.MaxIssueComments)

	for _, issue := range issues {
		comment := &github.PullRequestComment{
			Body:     github.String(issue.Issue),
			CommitID: github.String(r.commit),
			Path:     github.String(issue.Path),
			Position: github.Int(issue.HunkPos),
		}
		_, _, err := r.client.PullRequests.CreateComment(ctx, r.owner, r.repo, r.number, comment)
		if err != nil {
			return errors.Wrapf(err, "could not post comment path: %q, position: %v, commitID: %q, body: %q",
				*comment.Path, *comment.Position, *comment.CommitID, *comment.Body,
			)
		}
	}

	return nil
}

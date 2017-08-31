package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/bradleyfalzon/gopherci/internal/analyser"
	"github.com/bradleyfalzon/gopherci/internal/db"
	"github.com/bradleyfalzon/gopherci/internal/logger"
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

// dedupePRIssues deduplicates issues by checking the existing pull request for
// existing comments and returns comments that don't already exist.
func dedupePRIssues(ctx context.Context, client *github.Client, owner, repo string, number int, issues []db.Issue) (filtered []db.Issue, err error) {
	ecomments, _, err := client.PullRequests.ListComments(ctx, owner, repo, number, nil)
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
	filtered, err := dedupePRIssues(ctx, r.client, r.owner, r.repo, r.number, issues)
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

// StatusState is the state of a GitHub Status API as defined in
// https://developer.github.com/v3/repos/statuses/
type StatusState string

// https://developer.github.com/v3/repos/statuses/
const (
	StatusStatePending StatusState = "pending"
	StatusStateSuccess StatusState = "success"
	StatusStateError   StatusState = "error"
	StatusStateFailure StatusState = "failure"
)

// StatusAPIReporter uses the GitHub Statuses API to report build status, such
// as success or failure.
type StatusAPIReporter struct {
	logger    logger.Logger
	client    *github.Client
	statusURL string
	context   string
	targetURL string
}

var _ analyser.Reporter = &StatusAPIReporter{}

// NewStatusAPIReporter returns a StatusAPIReporter.
func NewStatusAPIReporter(logger logger.Logger, client *github.Client, statusURL, context, targetURL string) *StatusAPIReporter {
	return &StatusAPIReporter{
		logger:    logger,
		client:    client,
		statusURL: statusURL,
		context:   context,
		targetURL: targetURL,
	}
}

// SetStatus sets the CI Status API
func (r *StatusAPIReporter) SetStatus(ctx context.Context, status StatusState, description string) error {
	s := struct {
		State       string `json:"state,omitempty"`
		TargetURL   string `json:"target_url,omitempty"`
		Description string `json:"description,omitempty"`
		Context     string `json:"context,omitempty"`
	}{
		string(status), r.targetURL, description, r.context,
	}

	r.logger.Infof("Setting %v state: %q, context: %q, description: %q", r.statusURL, status, r.context, description)

	js, err := json.Marshal(&s)
	if err != nil {
		return errors.Wrap(err, "could not marshal status")
	}

	req, err := http.NewRequest("POST", r.statusURL, bytes.NewBuffer(js))
	if err != nil {
		return errors.Wrapf(err, "could not make status request")
	}
	resp, err := r.client.Do(ctx, req, nil)
	if err != nil {
		return errors.Wrapf(err, "could not set status to %s for %s", status, r.statusURL)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("received status code %d from %s", resp.StatusCode, r.statusURL)
	}
	return nil
}

// Report implements the analyser.Reporter interface.
func (r *StatusAPIReporter) Report(ctx context.Context, issues []db.Issue) error {
	// TODO remove suppressed count, we don't know how many were suppressed.
	suppressed, _ := analyser.Suppress(issues, analyser.MaxIssueComments)
	return r.SetStatus(ctx, StatusStateSuccess, r.statusDesc(issues, suppressed))
}

// statusDesc builds a status description based on issues.
func (StatusAPIReporter) statusDesc(issues []db.Issue, suppressed int) string {
	desc := fmt.Sprintf("Found %d issues", len(issues))
	switch {
	case len(issues) == 0:
		return `Found no issues \ʕ◔ϖ◔ʔ/`
	case len(issues) == 1:
		return `Found 1 issue`
	case suppressed == 1:
		desc += fmt.Sprintf(" (%v comment suppressed)", suppressed)
	case suppressed > 1:
		desc += fmt.Sprintf(" (%v comments suppressed)", suppressed)
	}
	return desc
}

// CommitCommentReporter creates a single commit comment summarising all issues
// on a given owner, repo, and commit hash.
type CommitCommentReporter struct {
	client      *github.Client
	owner       string
	repo        string
	commit      string
	commits     int
	analysisURL string
}

var _ analyser.Reporter = &CommitCommentReporter{}

// NewCommitCommentReporter returns a CommitCommentReporter. commits is the
// number of commits the analysis is checking (to be displayed in the message)
// and analysisURL is the URL of the analysis.
func NewCommitCommentReporter(client *github.Client, owner, repo, commit string, commits int, analysisURL string) *CommitCommentReporter {
	return &CommitCommentReporter{
		client:      client,
		owner:       owner,
		repo:        repo,
		commit:      commit,
		commits:     commits,
		analysisURL: analysisURL,
	}
}

// Report implements the analyser.Reporter interface.
func (r *CommitCommentReporter) Report(ctx context.Context, issues []db.Issue) error {
	if len(issues) == 0 {
		return nil
	}

	// msg assumes there's always 2 or more commits, else you'd use InlineCommitCommentReporter
	plural := ""
	if len(issues) > 1 {
		plural = "s"
	}
	msg := fmt.Sprintf("GopherCI found **%d** issue%s in the last **%d** commits, see: %s",
		len(issues), plural, r.commits, r.analysisURL,
	)

	comment := &github.RepositoryComment{
		Body: github.String(msg),
	}
	_, _, err := r.client.Repositories.CreateComment(ctx, r.owner, r.repo, r.commit, comment)
	return errors.Wrapf(err, "could not post comment commit: %q, body: %q", r.commit, *comment.Body)
}

// InlineCommitCommentReporter is a analyser.Reporter that creates a commit
// comment for each issue on a single commit. This should only be used if all
// issues occur on a single commit (not when an analysis checks multiple commits
// such as during a push for 2 or more commits).
type InlineCommitCommentReporter struct {
	client *github.Client
	owner  string
	repo   string
	commit string
}

var _ analyser.Reporter = &InlineCommitCommentReporter{}

// NewInlineCommitCommentReporter returns a InlineCommitCommentReporter.
func NewInlineCommitCommentReporter(client *github.Client, owner, repo, commit string) *InlineCommitCommentReporter {
	return &InlineCommitCommentReporter{
		client: client,
		owner:  owner,
		repo:   repo,
		commit: commit,
	}
}

// Report implements the analyser.Reporter interface.
func (r *InlineCommitCommentReporter) Report(ctx context.Context, issues []db.Issue) error {
	_, issues = analyser.Suppress(issues, analyser.MaxIssueComments)

	for _, issue := range issues {
		comment := &github.RepositoryComment{
			Body:     github.String(issue.Issue),
			Path:     github.String(issue.Path),
			Position: github.Int(issue.HunkPos),
		}
		_, _, err := r.client.Repositories.CreateComment(ctx, r.owner, r.repo, r.commit, comment)
		if err != nil {
			return errors.Wrapf(err, "could not post comment path: %q, position: %v, commit: %q, body: %q",
				*comment.Path, *comment.Position, r.commit, *comment.Body,
			)
		}
	}

	return nil
}

// PRReviewReporter is a analyser.Reporter that creates a pull request review
// on a given owner, repo, pr and commit hash. Sets review status to COMMENT
// if there are comments.
type PRReviewReporter struct {
	client *github.Client
	owner  string
	repo   string
	number int
	commit string
}

var _ analyser.Reporter = &PRReviewReporter{}

// NewPRReviewReporter returns a PRReviewReporter.
func NewPRReviewReporter(client *github.Client, owner, repo string, number int, commit string) *PRReviewReporter {
	return &PRReviewReporter{
		client: client,
		owner:  owner,
		repo:   repo,
		number: number,
		commit: commit,
	}
}

// Report implements the analyser.Reporter interface.
func (r *PRReviewReporter) Report(ctx context.Context, issues []db.Issue) error {
	issues, err := dedupePRIssues(ctx, r.client, r.owner, r.repo, r.number, issues)
	if err != nil {
		return err
	}

	_, issues = analyser.Suppress(issues, analyser.MaxIssueComments)

	if len(issues) == 0 {
		return nil
	}

	var comments []*github.DraftReviewComment
	for _, issue := range issues {
		comments = append(comments, &github.DraftReviewComment{
			Body:     github.String(issue.Issue),
			Path:     github.String(issue.Path),
			Position: github.Int(issue.HunkPos),
		})
	}

	_, _, err = r.client.PullRequests.CreateReview(ctx, r.owner, r.repo, r.number, &github.PullRequestReviewRequest{
		Event:    github.String("COMMENT"),
		CommitID: github.String(r.commit),
		Comments: comments,
	})
	return errors.Wrap(err, "could not post review")
}

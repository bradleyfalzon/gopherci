package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"

	"github.com/bradleyfalzon/gopherci/internal/db"
	"github.com/google/go-github/github"
	"github.com/pkg/errors"
)

// Installation is a GitHub Integration which has operates in the context of a
// GitHub installation, and therefore performance operations as that
// installation.
type Installation struct {
	ID     int
	client *github.Client
}

func (g *GitHub) NewInstallation(installationID int) (*Installation, error) {

	// TODO reuse installations, so we maintain rate limit state between webhooks
	installation, err := g.db.GetGHInstallation(installationID)
	if err != nil {
		return nil, err
	}
	if installation == nil {
		return nil, nil
	}
	if !installation.IsEnabled() {
		log.Printf("ignoring disabled installation: %+v", installation)
		return nil, nil
	}

	log.Printf("found installation: %+v", installation)
	itr, err := g.newInstallationTransport(installation.InstallationID)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("could not initialise transport for installation id %v", installation.InstallationID))
	}
	client := github.NewClient(&http.Client{Transport: itr})

	// Allow overwriting of baseURL for tests
	if client.BaseURL, err = url.Parse(g.baseURL); err != nil {
		return nil, err
	}

	return &Installation{ID: installation.ID, client: client}, nil
}

// StatusState is the state of a GitHub Status API as defined in
// https://developer.github.com/v3/repos/statuses/
type StatusState string

const (
	StatusStatePending StatusState = "pending"
	StatusStateSuccess StatusState = "success"
	StatusStateError   StatusState = "error"
	StatusStateFailure StatusState = "failure"
)

// SetStatus sets the CI Status API
func (i *Installation) SetStatus(ctx context.Context, context, statusURL string, status StatusState, description, targetURL string) error {
	s := struct {
		State       string `json:"state,omitempty"`
		TargetURL   string `json:"target_url,omitempty"`
		Description string `json:"description,omitempty"`
		Context     string `json:"context,omitempty"`
	}{
		string(status), targetURL, description, context,
	}
	log.Printf("status: %#v", status)

	js, err := json.Marshal(&s)
	if err != nil {
		return errors.Wrap(err, "could not marshal status")
	}

	req, err := http.NewRequest("POST", statusURL, bytes.NewBuffer(js))
	if err != nil {
		return err
	}
	resp, err := i.client.Do(ctx, req, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("received status code %v", resp.StatusCode)
	}
	return nil
}

// maxIssueComments is the maximum number of comments that will be written
// on a pull request by writeissues. a pr may have more comments written if
// writeissues is called multiple times, such is multiple syncronise events.
const maxIssueComments = 10

// FilterIssues deduplicates issues by checking the existing pull request for
// existing comments and returns comments that don't already exist.
// Additionally, only a maximum amount of issues will be returned, the number
// of total suppressed comments is returned.
func (i *Installation) FilterIssues(ctx context.Context, owner, repo string, prNumber int, issues []db.Issue) (suppressed int, filtered []db.Issue, err error) {
	ecomments, _, err := i.client.PullRequests.ListComments(ctx, owner, repo, prNumber, nil)
	if err != nil {
		return 0, nil, errors.Wrap(err, "could not list existing comments")
	}
	// remove duplicate comments, as we're remove elements based on the index
	// start from last position and work backwards to keep indexes consistent
	// even after removing elements.
	for i := len(issues) - 1; i >= 0; i-- {
		issue := issues[i]
		for _, ec := range ecomments {
			if issue.Path == *ec.Path && issue.HunkPos == *ec.Position && issue.Issue == *ec.Body {
				issues = append(issues[:i], issues[i+1:]...)
				break
			}
		}
	}
	// Of the de-duplicated issues, only return maxIssuesComments
	if len(issues) > maxIssueComments {
		return len(issues) - maxIssueComments, issues[:maxIssueComments], nil
	}
	return 0, issues, nil
}

// WriteIssues takes a slice of issues and creates a pull request comment for
// each issue on a given owner, repo, pr and commit hash. Returns on the first
// error encountered.
func (i *Installation) WriteIssues(ctx context.Context, owner, repo string, prNumber int, commit string, issues []db.Issue) error {
	for _, issue := range issues {
		comment := &github.PullRequestComment{
			Body:     github.String(issue.Issue),
			CommitID: github.String(commit),
			Path:     github.String(issue.Path),
			Position: github.Int(issue.HunkPos),
		}
		_, _, err := i.client.PullRequests.CreateComment(ctx, owner, repo, prNumber, comment)
		if err != nil {
			return errors.Wrap(err, "could not post comment")
		}
	}
	return nil
}

// Diff implements the web.VCSReader interface.
func (i *Installation) Diff(ctx context.Context, repositoryID int, commitFrom, commitTo string, requestNumber int) (io.ReadCloser, error) {
	var apiURL string
	if requestNumber == 0 {
		apiURL = fmt.Sprintf("%s/repositories/%d/compare/%s...%s", i.client.BaseURL.String(), repositoryID, commitFrom, commitTo)
	} else {
		apiURL = fmt.Sprintf("%s/repositories/%d/pulls/%d", i.client.BaseURL.String(), repositoryID, requestNumber)
	}

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}

	var js struct {
		DiffURL string `json:"diff_url"`
	}
	_, err = i.client.Do(ctx, req, &js)
	if err != nil {
		return nil, err
	}

	if js.DiffURL == "" {
		return nil, errors.New("no diff url in api response")
	}

	resp, err := http.Get(js.DiffURL)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

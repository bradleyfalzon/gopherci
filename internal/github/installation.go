package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"

	"github.com/bradleyfalzon/gopherci/internal/analyser"
	"github.com/google/go-github/github"
	"github.com/pkg/errors"
)

type Installation struct {
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

	return &Installation{client: client}, nil
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
func (i *Installation) SetStatus(statusURL string, status StatusState, description string) error {
	s := struct {
		State       string `json:"state,omitempty"`
		TargetURL   string `json:"target_url,omitempty"`
		Description string `json:"description,omitempty"`
		Context     string `json:"context,omitempty"`
	}{
		string(status), "", description, "ci/gopherci",
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
	resp, err := i.client.Do(req, nil)
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

// WriteIssues takes a slice of issues and creates a pull request comment for
// each issue on a given owner, repo, pr and commit hash. Returns on the first
// error encountered.
func (i *Installation) WriteIssues(owner, repo string, prNumber int, commit string, issues []analyser.Issue) (suppressed int, err error) {
	for n, issue := range issues {
		if n >= maxIssueComments {
			suppressed = len(issues) - maxIssueComments
			break
		}
		comment := &github.PullRequestComment{
			Body:     github.String(issue.Issue),
			CommitID: github.String(commit),
			Path:     github.String(issue.File),
			Position: github.Int(issue.HunkPos),
		}
		_, resp, err := i.client.PullRequests.CreateComment(owner, repo, prNumber, comment)
		if err != nil {
			return suppressed, errors.Wrapf(err, "github api response rate: %v", resp.Rate)
		}
	}
	return suppressed, nil
}

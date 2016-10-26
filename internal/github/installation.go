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

// accountID is the repo owner's id
func (g *GitHub) NewInstallation(accountID int) (*Installation, error) {

	// TODO reuse installations, so we maintain rate limit state between webhooks
	dbInstallation, err := g.db.FindGHInstallation(accountID)
	if err != nil {
		return nil, err
	}
	if dbInstallation == nil {
		return nil, nil
	}

	log.Printf("found installation: %+v", dbInstallation)
	itr, err := g.newInstallationTransport(dbInstallation.InstallationID)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("could not initialise transport for installation id %v", dbInstallation.InstallationID))
	}
	client := github.NewClient(&http.Client{Transport: itr})

	// Allow overwriting of baseURL for tests
	if client.BaseURL, err = url.Parse(g.baseURL); err != nil {
		return nil, err
	}

	return &Installation{client: client}, nil
}

type StatusState string

const (
	StatusStatePending StatusState = "pending"
	StatusStateSuccess             = "success"
	StatusStateError               = "error"
	StatusStateFailure             = "failure"
)

// SetStatus sets the CI Status API
func (i *Installation) SetStatus(statusURL string, status StatusState) error {

	// Set the CI status API to pending
	s := struct {
		State       string `json:"state,omitempty"`
		TargetURL   string `json:"target_url,omitempty"`
		Description string `json:"description,omitempty"`
		Context     string `json:"context,omitempty"`
	}{
		string(status), "", "short description", "continuous-integration/gopherci",
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

func (i *Installation) WriteIssues(prNumber int, commit string, issues []analyser.Issue) {
	// TODO make this idempotent, so don't post the same issue twice
	// which may occur when we support additional commits to a PR (synchronize
	// api event)
	for _, issue := range issues {
		comment := &github.PullRequestComment{
			Body:     github.String(issue.Issue),
			CommitID: github.String(commit),
			Path:     github.String(issue.File),
			Position: github.Int(issue.HunkPos),
		}

		cmt, resp, err := i.client.PullRequests.CreateComment("bf-test", "gopherci-dev1", prNumber, comment)
		log.Print("cmt:", cmt)
		log.Print("resp:", resp)
		log.Print("err:", err)
	}
}

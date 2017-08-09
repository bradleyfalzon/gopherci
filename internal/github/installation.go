package github

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"

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

// IsEnabled returns true if an installation is enabled.
func (i *Installation) IsEnabled() bool {
	return i != nil
}

// Diff implements the web.VCSReader interface.
func (i *Installation) Diff(ctx context.Context, repositoryID int, commitFrom, commitTo string, requestNumber int) (io.ReadCloser, error) {
	var apiURL string
	switch {
	case requestNumber != 0:
		apiURL = fmt.Sprintf("%s/repositories/%d/pulls/%d", i.client.BaseURL.String(), repositoryID, requestNumber)
	case commitFrom == "":
		// There doesn't appear to be an API call which returns a diff for the
		// first commit in a repository.
		return nil, nil
	default:
		apiURL = fmt.Sprintf("%s/repositories/%d/compare/%s...%s", i.client.BaseURL.String(), repositoryID, commitFrom, commitTo)
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
		return nil, fmt.Errorf("no diff url in api: %v", apiURL)
	}

	resp, err := http.Get(js.DiffURL)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

package github

import (
	"net/http"

	"github.com/bradleyfalzon/ghinstallation"
	"github.com/bradleyfalzon/gopherci/internal/analyser"
	"github.com/bradleyfalzon/gopherci/internal/db"
)

const (
	// acceptHeader is the GitHub Integrations Preview Accept header.
	acceptHeader = "application/vnd.github.machine-man-preview+json"
)

// GitHub is the type gopherci uses to interract with github.com.
type GitHub struct {
	db             db.DB
	analyser       analyser.Analyser
	integrationID  int               // id is the integration id
	integrationKey []byte            // integrationKey is the private key for the installationID
	tr             http.RoundTripper // tr is a transport shared by all installations to reuse http connections
	baseURL        string            // baseURL for GitHub API
}

// New returns a GitHub object for use with GitHub integrations
// https://developer.github.com/changes/2016-09-14-Integrations-Early-Access/
// integrationID is the GitHub Integration ID (not installation ID).
// integrationKey is the key for the integrationID provided to you by GitHub
// during the integration registration.
func New(analyser analyser.Analyser, db db.DB, integrationID int, integrationKey []byte) (*GitHub, error) {
	g := &GitHub{
		analyser:       analyser,
		db:             db,
		integrationID:  integrationID,
		integrationKey: integrationKey,
		tr:             http.DefaultTransport,
		baseURL:        "https://api.github.com",
	}

	// TODO some prechecks should be done now, instead of later, fail fast/early.

	return g, nil
}

func (g *GitHub) newInstallationTransport(installationID int) (*ghinstallation.Transport, error) {
	tr, err := ghinstallation.New(g.tr, g.integrationID, installationID, g.integrationKey)
	if err != nil {
		return nil, err
	}
	tr.BaseURL = g.baseURL
	return tr, nil
}

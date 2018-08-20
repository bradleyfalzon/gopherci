package github

import (
	"net/http"

	"github.com/bradleyfalzon/ghinstallation"
	"github.com/bradleyfalzon/gopherci/internal/analyser"
	"github.com/bradleyfalzon/gopherci/internal/db"
	"github.com/bradleyfalzon/gopherci/internal/logger"
	"github.com/sethgrid/pester"
)

// GitHub is the type gopherci uses to interract with github.com.
type GitHub struct {
	logger         logger.Logger
	db             db.DB
	analyser       analyser.Analyser
	queuePush      chan<- interface{}
	webhookSecret  []byte            // shared webhook secret configured for the integration
	integrationID  int64             // id is the integration id
	integrationKey []byte            // integrationKey is the private key for the installationID
	tr             http.RoundTripper // tr is a transport shared by all installations to reuse http connections
	baseURL        string            // baseURL for GitHub API
	gciBaseURL     string            // gciBaseURL is the base URL for GopherCI
}

// New returns a GitHub object for use with GitHub integrations
// https://developer.github.com/changes/2016-09-14-Integrations-Early-Access/
// integrationID is the GitHub Integration ID (not installation ID).
// integrationKey is the key for the integrationID provided to you by GitHub
// during the integration registration.
func New(logger logger.Logger, analyser analyser.Analyser, db db.DB, queuePush chan<- interface{}, integrationID int64, integrationKey []byte, webhookSecret, gciBaseURL string) (*GitHub, error) {
	g := &GitHub{
		logger:         logger,
		analyser:       analyser,
		db:             db,
		queuePush:      queuePush,
		webhookSecret:  []byte(webhookSecret),
		integrationID:  integrationID,
		integrationKey: integrationKey,
		tr:             http.DefaultTransport,
		baseURL:        "https://api.github.com",
		gciBaseURL:     gciBaseURL,
	}

	// TODO some prechecks should be done now, instead of later, fail fast/early.

	return g, nil
}

func (g *GitHub) newInstallationTransport(installationID int64) (*ghinstallation.Transport, error) {
	tr, err := ghinstallation.New(g.tr, int(g.integrationID), int(installationID), g.integrationKey)
	if err != nil {
		return nil, err
	}
	tr.Client = pester.New() // provide retry functionality for intermittent network issues
	tr.BaseURL = g.baseURL
	return tr, nil
}

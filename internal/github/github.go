package github

import (
	"net/http"
	"strconv"

	"github.com/pkg/errors"

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
	db            db.DB
	analyser      analyser.Analyser
	integrationID int               // id is the integration id
	keyFile       string            // keyFile is the path to private key
	tr            http.RoundTripper // tr is a transport shared by all installations to reuse http connections
	baseURL       string            // baseURL for GitHub API
}

// New returns a GitHub object for use with GitHub integrations
// https://developer.github.com/changes/2016-09-14-Integrations-Early-Access/
// integrationID is the GitHub Integration ID (not installation ID), keyFile is the path to the
// private key provided to you by GitHub during the integration registration.
func New(analyser analyser.Analyser, db db.DB, integrationID, keyFile string) (*GitHub, error) {
	iid, err := strconv.ParseInt(integrationID, 10, 64)
	if err != nil {
		return nil, errors.Wrap(err, "could not parse integrationID")
	}

	g := &GitHub{
		analyser:      analyser,
		db:            db,
		integrationID: int(iid),
		keyFile:       keyFile,
		tr:            http.DefaultTransport,
		baseURL:       "https://api.github.com",
	}

	// TODO some prechecks should be done now, instead of later, fail fast/early.

	return g, nil
}

func (g *GitHub) newInstallationTransport(installationID int) (*ghinstallation.Transport, error) {
	tr, err := ghinstallation.NewKeyFromFile(g.tr, g.integrationID, installationID, g.keyFile)
	if err != nil {
		return nil, err
	}
	tr.BaseURL = g.baseURL
	return tr, nil
}

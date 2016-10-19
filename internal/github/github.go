package github

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"github.com/pkg/errors"

	"github.com/bradleyfalzon/gopherci/internal/analyser"
	"github.com/bradleyfalzon/gopherci/internal/db"
	"github.com/dgrijalva/jwt-go"
	"github.com/google/go-github/github"
)

const (
	// acceptHeader is the GitHub Integrations Preview Accept header.
	acceptHeader = "application/vnd.github.machine-man-preview+json"
)

// GitHub is the type gopherci uses to interract with github.com.
type GitHub struct {
	db            *db.DB
	analyser      analyser.Analyser
	integrationID string            // id is the integration id
	keyFile       string            // keyFile is the path to private key
	tr            http.RoundTripper // tr is a transport shared by all installations to reuse http connections
}

// New returns a GitHub object for use with GitHub integrations
// https://developer.github.com/changes/2016-09-14-Integrations-Early-Access/
// integrationID is the GitHub Integration ID (not installation ID), keyFile is the path to the
// private key provided to you by GitHub during the integration registration.
func New(analyser analyser.Analyser, db *db.DB, integrationID, keyFile string) (*GitHub, error) {
	g := &GitHub{
		analyser:      analyser,
		db:            db,
		integrationID: integrationID,
		keyFile:       keyFile,
		tr:            http.DefaultTransport,
	}

	// TODO some prechecks should be done now, instead of later, fail fast/early.

	return g, nil
}

// writeComment is just an example of how to use the installation transport
func (g *GitHub) writeComment() {

	itr := g.newInstallationTransport(g.tr, "1859")
	httpClient := &http.Client{Transport: itr}
	ghClient := github.NewClient(httpClient)

	comment := &github.PullRequestComment{
		Body:     github.String("this is a body"),
		CommitID: github.String("3c9b0fa6a2ff9e388187b3001710a3b4d4062024"),
		Path:     github.String("main.go"),
		Position: github.Int(7),
	}

	cmt, resp, err := ghClient.PullRequests.CreateComment("bf-test", "gopherci-dev1", 16, comment)
	log.Print(cmt)
	log.Print(resp)
	log.Print(err)

}

// installationTransport provides a http.RoundTripper by wrapping an existing
// http.RoundTripper (that's shared between multiple installation transports to
// reuse underlying http connections), but provides GitHub Integration
// authentication as an installation.
//
// See https://developer.github.com/early-access/integrations/authentication/#as-an-installation
type installationTransport struct {
	tr             http.RoundTripper // tr is the underlying roundtripper being wrapped
	keyFile        string            // keyFile is the path to GitHub Intregration's PEM encoded private key
	integrationID  string            // integrationID is the GitHub Integration's Installation ID
	installationID string            // installationID is the GitHub Integration's Installation ID
	token          *accessToken      // token is the installation's access token
}

// accessToken is an installation access token
type accessToken struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (g *GitHub) newInstallationTransport(tr http.RoundTripper, installationID string) *installationTransport {
	return &installationTransport{
		tr:             tr,
		keyFile:        g.keyFile,
		integrationID:  g.integrationID,
		installationID: installationID,
	}
}

func (t *installationTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.token == nil || t.token.ExpiresAt.Add(-time.Minute).Before(time.Now()) {
		// Token is not set or expired/nearly expired, so refresh
		if err := t.refreshToken(); err != nil {
			return nil, errors.Wrap(err, "could not refresh installation token")
		}
	}

	req.Header.Set("Authorization", "token "+t.token.Token)
	req.Header.Set("Accept", acceptHeader)
	resp, err := t.tr.RoundTrip(req)
	return resp, err
}

func (t *installationTransport) refreshToken() error {
	// TODO these claims could probably be reused between installations before expiry
	claims := &jwt.StandardClaims{
		IssuedAt:  time.Now().Unix(),
		ExpiresAt: time.Now().Add(time.Minute).Unix(),
		Issuer:    t.integrationID,
	}
	bearer := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)

	key, err := ioutil.ReadFile(t.keyFile)
	if err != nil {
		return errors.Wrap(err, "could not read private key")
	}

	// discard extra bytes in the file, we only care about the key
	block, _ := pem.Decode(key)
	if block == nil {
		return errors.New("could not decode pem private key")
	}

	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return errors.Wrap(err, "could not parse private key")
	}

	ss, err := bearer.SignedString(privateKey)
	if err != nil {
		return errors.Wrap(err, "could not sign jwt")
	}

	req, err := http.NewRequest("POST", fmt.Sprintf("https://api.github.com/installations/%v/access_tokens", t.installationID), nil)
	if err != nil {
		return errors.Wrap(err, "could not create request")
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %v", ss))
	req.Header.Set("Accept", acceptHeader)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return errors.Wrap(err, "could not get access_tokens")
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(&t.token); err != nil {
		return err
	}

	return nil
}

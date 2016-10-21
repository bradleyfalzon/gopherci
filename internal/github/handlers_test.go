package github

import (
	"errors"
	"testing"

	"github.com/bradleyfalzon/gopherci/internal/analyser"
	"github.com/bradleyfalzon/gopherci/internal/db"
	"github.com/google/go-github/github"
)

func setup(t *testing.T) (*GitHub, *db.MemDB) {
	memDB := db.NewMemDB()
	nopAnalyser := analyser.NewNOP()

	//New GitHub
	g, err := New(nopAnalyser, memDB, "1", "")
	if err != nil {
		t.Fatal("could not initialise GitHub:", err)
	}
	return g, memDB
}

func TestWebHook_integrationInstallation(t *testing.T) {
	g, memDB := setup(t)

	const (
		accountID      = 1
		installationID = 2
	)

	installation := &github.Installation{
		ID: github.Int(installationID),
		Account: &github.User{
			ID:    github.Int(accountID),
			Login: github.String("accountlogin"),
		},
	}

	event := &github.IntegrationInstallationEvent{
		Action:       github.String("created"),
		Installation: installation,
		Sender: &github.User{
			Login: github.String("senderlogin"),
		},
	}

	// Send create event
	g.integrationInstallationEvent(event)

	// Check DB received it
	got, _ := memDB.FindGHInstallation(accountID)
	if got.InstallationID != installationID {
		t.Errorf("got: %#v, want %#v", got.InstallationID, installationID)
	}

	// Send delete event
	event.Action = github.String("deleted")
	g.integrationInstallationEvent(event)

	got, _ = memDB.FindGHInstallation(accountID)
	if got != nil {
		t.Errorf("got: %#v, expected nil", got)
	}

	// force error
	memDB.ForceError(errors.New("forced"))
	g.integrationInstallationEvent(event)
	memDB.ForceError(nil)
}

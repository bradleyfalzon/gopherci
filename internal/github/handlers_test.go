package github

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/bradleyfalzon/gopherci/internal/analyser"
	"github.com/bradleyfalzon/gopherci/internal/db"
	"github.com/google/go-github/github"
)

func setup(t *testing.T) (*GitHub, *analyser.MockAnalyser, *db.MemDB) {
	memDB := db.NewMemDB()
	mockAnalyser := &analyser.MockAnalyser{}

	// New GitHub
	g, err := New(mockAnalyser, memDB, "1", "test.pem")
	if err != nil {
		t.Fatal("could not initialise GitHub:", err)
	}
	return g, mockAnalyser, memDB
}

func TestIntegrationInstallationEvent(t *testing.T) {
	g, _, memDB := setup(t)

	const (
		accountID      = 1
		installationID = 2
	)

	event := &github.IntegrationInstallationEvent{
		Action: github.String("created"),
		Installation: &github.Installation{
			ID: github.Int(installationID),
			Account: &github.User{
				ID:    github.Int(accountID),
				Login: github.String("accountlogin"),
			},
		},
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

func TestPullRequestEvent(t *testing.T) {
	g, mockAnalyser, memDB := setup(t)

	var (
		statePending  bool
		stateSuccess  bool
		postedComment bool
	)

	var (
		expectedRepoURL = "some-repo-url"
		expectedBranch  = "some-branch"
		expectedDiffURL = "some-diff-url"
		expectedCmtBody = "some-issue"
		expectedCmtPath = "main.go"
		expectedCmtPos  = 1
		expectedCmtSHA  = "abcdef"
	)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decoder := json.NewDecoder(r.Body)
		switch r.RequestURI {
		case "/status-url":
			// Make sure status was set to pending and then success
			var status struct {
				State string `json:"state"`
			}
			err := decoder.Decode(&status)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			switch {
			case !statePending && !stateSuccess && status.State == string(StatusStatePending):
				statePending = true
			case statePending && !stateSuccess && status.State == string(StatusStateSuccess):
				stateSuccess = true
			default:
				t.Fatalf("unexpected status api change to %v %v %v", status.State, statePending, stateSuccess)
			}
		case "/installations/2/access_tokens":
			// respond with any token to installation transport
			js, _ := json.Marshal(accessToken{})
			fmt.Fprintln(w, string(js))
		case "/repos/bf-test/gopherci-dev1/pulls/1/comments":
			expected := github.PullRequestComment{
				Body:     github.String(expectedCmtBody),
				Path:     github.String(expectedCmtPath),
				Position: github.Int(expectedCmtPos),
				CommitID: github.String(expectedCmtSHA),
			}
			var comment github.PullRequestComment
			err := decoder.Decode(&comment)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(expected, comment) {
				t.Fatalf("expected cmt:\n%#v\ngot:\n%#v", expected, comment)
			} else {
				postedComment = true
			}
		default:
			t.Logf(r.RequestURI)
		}
	}))
	defer ts.Close()
	g.baseURL = ts.URL

	const (
		accountID      = 1
		installationID = 2
	)

	_ = memDB.AddGHInstallation(installationID, accountID)
	mockAnalyser.Issues = []analyser.Issue{{expectedCmtPath, expectedCmtPos, expectedCmtBody}}

	event := &github.PullRequestEvent{
		Action: github.String("opened"),
		Number: github.Int(1),
		PullRequest: &github.PullRequest{
			StatusesURL: github.String(ts.URL + "/status-url"),
			DiffURL:     github.String(expectedDiffURL),
			Head: &github.PullRequestBranch{
				Repo: &github.Repository{
					CloneURL: github.String(expectedRepoURL),
				},
				SHA: github.String(expectedCmtSHA),
				Ref: github.String(expectedBranch),
			},
		},
		Repo: &github.Repository{
			Owner: &github.User{
				ID: github.Int(accountID),
			},
		},
	}

	err := g.pullRequestEvent(event)
	switch {
	case err != nil:
		t.Errorf("did not expect error: %v", err)
	case !statePending:
		t.Errorf("did not set status state to pending")
	case mockAnalyser.RepoURL != expectedRepoURL:
		t.Errorf("analyser repoURL expected %v, got %v", expectedRepoURL, mockAnalyser.DiffURL)
	case mockAnalyser.Branch != expectedBranch:
		t.Errorf("analyser branch expected %v, got %v", expectedBranch, mockAnalyser.Branch)
	case mockAnalyser.DiffURL != expectedDiffURL:
		t.Errorf("analyser diffURL expected %v, got %v", expectedDiffURL, mockAnalyser.DiffURL)
	case !postedComment:
		t.Errorf("did not post comment")
	case !stateSuccess:
		t.Errorf("did not set status state to success")
	}
}

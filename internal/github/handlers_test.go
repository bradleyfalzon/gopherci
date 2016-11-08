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

// test integration key
var integrationKey = []byte(`-----BEGIN RSA PRIVATE KEY-----
MIIEpQIBAAKCAQEA0BUezcR7uycgZsfVLlAf4jXP7uFpVh4geSTY39RvYrAll0yh
q7uiQypP2hjQJ1eQXZvkAZx0v9lBYJmX7e0HiJckBr8+/O2kARL+GTCJDJZECpjy
97yylbzGBNl3s76fZ4CJ+4f11fCh7GJ3BJkMf9NFhe8g1TYS0BtSd/sauUQEuG/A
3fOJxKTNmICZr76xavOQ8agA4yW9V5hKcrbHzkfecg/sQsPMmrXixPNxMsqyOMmg
jdJ1aKr7ckEhd48ft4bPMO4DtVL/XFdK2wJZZ0gXJxWiT1Ny41LVql97Odm+OQyx
tcayMkGtMb1nwTcVVl+RG2U5E1lzOYpcQpyYFQIDAQABAoIBAAfUY55WgFlgdYWo
i0r81NZMNBDHBpGo/IvSaR6y/aX2/tMcnRC7NLXWR77rJBn234XGMeQloPb/E8iw
vtjDDH+FQGPImnQl9P/dWRZVjzKcDN9hNfNAdG/R9JmGHUz0JUddvNNsIEH2lgEx
C01u/Ntqdbk+cDvVlwuhm47MMgs6hJmZtS1KDPgYJu4IaB9oaZFN+pUyy8a1w0j9
RAhHpZrsulT5ThgCra4kKGDNnk2yfI91N9lkP5cnhgUmdZESDgrAJURLS8PgInM4
YPV9L68tJCO4g6k+hFiui4h/4cNXYkXnaZSBUoz28ICA6e7I3eJ6Y1ko4ou+Xf0V
csM8VFkCgYEA7y21JfECCfEsTHwwDg0fq2nld4o6FkIWAVQoIh6I6o6tYREmuZ/1
s81FPz/lvQpAvQUXGZlOPB9eW6bZZFytcuKYVNE/EVkuGQtpRXRT630CQiqvUYDZ
4FpqdBQUISt8KWpIofndrPSx6JzI80NSygShQsScWFw2wBIQAnV3TpsCgYEA3reL
L7AwlxCacsPvkazyYwyFfponblBX/OvrYUPPaEwGvSZmE5A/E4bdYTAixDdn4XvE
ChwpmRAWT/9C6jVJ/o1IK25dwnwg68gFDHlaOE+B5/9yNuDvVmg34PWngmpucFb/
6R/kIrF38lEfY0pRb05koW93uj1fj7Uiv+GWRw8CgYEAn1d3IIDQl+kJVydBKItL
tvoEur/m9N8wI9B6MEjhdEp7bXhssSvFF/VAFeQu3OMQwBy9B/vfaCSJy0t79uXb
U/dr/s2sU5VzJZI5nuDh67fLomMni4fpHxN9ajnaM0LyI/E/1FFPgqM+Rzb0lUQb
yqSM/ptXgXJls04VRl4VjtMCgYEAprO/bLx2QjxdPpXGFcXbz6OpsC92YC2nDlsP
3cfB0RFG4gGB2hbX/6eswHglLbVC/hWDkQWvZTATY2FvFps4fV4GrOt5Jn9+rL0U
elfC3e81Dw+2z7jhrE1ptepprUY4z8Fu33HNcuJfI3LxCYKxHZ0R2Xvzo+UYSBqO
ng0eTKUCgYEAxW9G4FjXQH0bjajntjoVQGLRVGWnteoOaQr/cy6oVii954yNMKSP
rezRkSNbJ8cqt9XQS+NNJ6Xwzl3EbuAt6r8f8VO1TIdRgFOgiUXRVNZ3ZyW8Hegd
kGTL0A6/0yAu9qQZlFbaD5bWhQo7eyx63u4hZGppBhkTSPikOYUPCH8=
-----END RSA PRIVATE KEY-----`)

type mockAnalyser struct {
	executed int
}

func (a *mockAnalyser) NewExecuter() (analyser.Executer, error) { return a, nil }
func (a *mockAnalyser) Execute(args []string) (out []byte, err error) {
	if len(args) > 0 && args[0] == "tool" {
		return []byte(`main.go:1: error`), nil
	}
	return nil, nil
}
func (a *mockAnalyser) Stop() error { return nil }

func setup(t *testing.T) (*GitHub, *db.MockDB) {
	memDB := db.NewMockDB()

	// New GitHub
	g, err := New(&mockAnalyser{}, memDB, 1, integrationKey)
	if err != nil {
		t.Fatal("could not initialise GitHub:", err)
	}
	return g, memDB
}

func TestIntegrationInstallationEvent(t *testing.T) {
	g, memDB := setup(t)

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
	g, memDB := setup(t)

	var (
		statePending  bool
		stateSuccess  bool
		postedComment bool
	)

	var (
		expectedConfig = analyser.Config{
			BaseURL:    "base-repo-url",
			BaseBranch: "base-branch",
			HeadURL:    "head-repo-url",
			HeadBranch: "head-branch",
		}
		expectedCmtBody = "Name: error"
		expectedCmtPath = "main.go"
		expectedCmtPos  = 1
		expectedCmtSHA  = "error"
	)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decoder := json.NewDecoder(r.Body)
		switch r.RequestURI {
		case "/diff-url":
			fmt.Fprintln(w, `diff --git a/subdir/main.go b/subdir/main.go
new file mode 100644
index 0000000..6362395
--- /dev/null
+++ b/main.go
@@ -0,0 +1,1 @@
+var _ = fmt.Sprintln()`)
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
			fmt.Fprintln(w, "{}")
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
	expectedConfig.DiffURL = ts.URL + "/diff-url"

	const (
		accountID      = 1
		installationID = 2
	)

	_ = memDB.AddGHInstallation(installationID, accountID)

	memDB.Tools = []db.Tool{
		{Name: "Name", Path: "tool", Args: "-flag %BASE_BRANCH% ./..."},
	}

	event := &github.PullRequestEvent{
		Action: github.String("opened"),
		Number: github.Int(1),
		PullRequest: &github.PullRequest{
			StatusesURL: github.String(ts.URL + "/status-url"),
			DiffURL:     github.String(expectedConfig.DiffURL),
			Base: &github.PullRequestBranch{
				Repo: &github.Repository{
					CloneURL: github.String(expectedConfig.BaseURL),
				},
				Ref: github.String(expectedConfig.BaseBranch),
			},
			Head: &github.PullRequestBranch{
				Repo: &github.Repository{
					CloneURL: github.String(expectedConfig.HeadURL),
				},
				SHA: github.String(expectedCmtSHA),
				Ref: github.String(expectedConfig.HeadBranch),
			},
		},
		Repo: &github.Repository{
			URL: github.String("repo-url"),
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
	case !postedComment:
		t.Errorf("did not post comment")
	case !stateSuccess:
		t.Errorf("did not set status state to success")
	}
}

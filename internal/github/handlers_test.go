package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"

	"github.com/bradleyfalzon/gopherci/internal/analyser"
	"github.com/bradleyfalzon/gopherci/internal/db"
	"github.com/bradleyfalzon/gopherci/internal/queue"
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
	goSrcPath string
}

func (a *mockAnalyser) NewExecuter(goSrcPath string) (analyser.Executer, error) {
	a.goSrcPath = goSrcPath
	return a, nil
}
func (a *mockAnalyser) Execute(args []string) (out []byte, err error) {
	if len(args) > 1 && args[0] == "git" && args[1] == "diff" {
		return []byte(`diff --git a/subdir/main.go b/subdir/main.go
new file mode 100644
index 0000000..6362395
--- /dev/null
+++ b/main.go
@@ -0,0 +1,1 @@
+var _ = fmt.Sprintln()`), nil
	}
	if len(args) > 0 && args[0] == "tool" {
		return []byte(`main.go:1: error`), nil
	}
	if len(args) > 0 && args[0] == "isFileGenerated" {
		return nil, &analyser.NonZeroError{ExitCode: 1}
	}
	return nil, nil
}
func (a *mockAnalyser) Stop() error { return nil }

const webhookSecret = "ede9aa6b6e04fafd53f7460fb75644302e249177"

func setup(t *testing.T) (*GitHub, *mockAnalyser, *db.MockDB) {
	memDB := db.NewMockDB()
	mockAnalyser := &mockAnalyser{}
	var (
		wg sync.WaitGroup
		c  = make(chan interface{})
	)
	queue := queue.NewMemoryQueue(context.Background(), &wg, c)

	// New GitHub
	g, err := New(mockAnalyser, memDB, queue, 1, integrationKey, webhookSecret)
	if err != nil {
		t.Fatal("could not initialise GitHub:", err)
	}
	return g, mockAnalyser, memDB
}

func TestWebhookHandler(t *testing.T) {
	tests := []struct {
		signature  string
		event      string
		expectCode int
	}{
		{"sha1=d1e100e3f17e8399b73137382896ff1536c59457", "goci-invalid", http.StatusBadRequest},
		{"sha1=d1e100e3f17e8399b73137382896ff1536c59457", "issues", http.StatusOK},
		{"sha1=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "issues", http.StatusBadRequest},
	}

	for _, test := range tests {
		g, _, _ := setup(t)
		body := bytes.NewBufferString(`{"key":"value"}`)
		r, err := http.NewRequest("POST", "https://example.com", body)
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Add("X-GitHub-Event", test.event)
		r.Header.Add("X-Hub-Signature", test.signature)
		w := httptest.NewRecorder()
		g.WebHookHandler(w, r)

		if w.Code != test.expectCode {
			t.Fatalf("have code: %v, want: %v, test: %+v", w.Code, test.expectCode, test)
		}
	}
}

func TestIntegrationInstallationEvent(t *testing.T) {
	g, _, memDB := setup(t)

	const (
		installationID = 2
		accountID      = 3
		senderID       = 4
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
			ID:    github.Int(senderID),
			Login: github.String("senderlogin"),
		},
	}

	// Send create event
	g.integrationInstallationEvent(event)

	want := &db.GHInstallation{
		InstallationID: installationID,
		AccountID:      accountID,
		SenderID:       senderID,
	}

	// Check DB received it
	have, _ := memDB.GetGHInstallation(installationID)
	if !reflect.DeepEqual(have, want) {
		t.Errorf("\nhave: %#v\nwant: %#v", have, want)
	}

	// Send delete event
	event.Action = github.String("deleted")
	g.integrationInstallationEvent(event)

	have, _ = memDB.GetGHInstallation(installationID)
	if have != nil {
		t.Errorf("got: %#v, expected nil", have)
	}

	// force error
	memDB.ForceError(errors.New("forced"))
	g.integrationInstallationEvent(event)
	memDB.ForceError(nil)
}

func TestPushConfig(t *testing.T) {
	want := AnalyseConfig{
		eventType:       analyser.EventTypePush,
		installationID:  1,
		statusesContext: "ci/gopherci/push",
		statusesURL:     "https://github.com/owner/repo/status/abcdef",
		baseURL:         "https://github.com/owner/repo.git",
		baseRef:         "abcdef~2",
		headURL:         "https://github.com/owner/repo.git",
		headRef:         "abcdef",
		goSrcPath:       "github.com/owner/repo",
	}
	e := &github.PushEvent{
		Installation: &github.Installation{
			ID: github.Int(1),
		},
		Repo: &github.PushEventRepository{
			StatusesURL: github.String("https://github.com/owner/repo/status/{sha}"),
			CloneURL:    github.String("https://github.com/owner/repo.git"),
			HTMLURL:     github.String("https://github.com/owner/repo"),
		},
		After:   github.String("abcdef"),
		Commits: []github.PushEventCommit{{}, {}},
	}

	have := PushConfig(e)
	if have != want {
		t.Errorf("have:\n%+v\nwant:\n%+v", have, want)
	}

}

func TestPullRequestConfig(t *testing.T) {
	want := AnalyseConfig{
		eventType:       analyser.EventTypePullRequest,
		installationID:  1,
		statusesContext: "ci/gopherci/pr",
		statusesURL:     "https://github.com/owner/repo/status/abcdef",
		baseURL:         "https://github.com/owner/repo.git",
		baseRef:         "base-branch",
		headURL:         "https://github.com/owner/repo.git",
		headRef:         "head-branch",
		goSrcPath:       "github.com/owner/repo",
		owner:           "owner",
		repo:            "repo",
		pr:              2,
		sha:             "abcdef",
	}
	e := &github.PullRequestEvent{
		Action: github.String("opened"),
		Number: github.Int(2),
		PullRequest: &github.PullRequest{
			StatusesURL: github.String("https://github.com/owner/repo/status/abcdef"),
			Base: &github.PullRequestBranch{
				Repo: &github.Repository{
					HTMLURL:  github.String("https://github.com/owner/repo"),
					CloneURL: github.String("https://github.com/owner/repo.git"),
					Name:     github.String("repo"),
					Owner: &github.User{
						Login: github.String("owner"),
					},
				},
				Ref: github.String("base-branch"),
			},
			Head: &github.PullRequestBranch{
				Repo: &github.Repository{
					CloneURL: github.String("https://github.com/owner/repo.git"),
				},
				SHA: github.String("abcdef"),
				Ref: github.String("head-branch"),
			},
		},
		Installation: &github.Installation{
			ID: github.Int(1),
		},
	}
	have := PullRequestConfig(e)
	if have != want {
		t.Errorf("have:\n%+v\nwant:\n%+v", have, want)
	}
}

func TestAnalyse(t *testing.T) {
	g, mockAnalyser, memDB := setup(t)

	var (
		statePending  bool
		stateSuccess  bool
		postedComment bool
	)

	var (
		expectedCmtBody   = "Name: error"
		expectedCmtPath   = "main.go"
		expectedCmtPos    = 1
		expectedCmtSHA    = "error"
		expectedOwner     = "owner"
		expectedRepo      = "repo"
		expectedPR        = 3
		expectedGoSrcPath = "github.com/owner/repo"
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
			fmt.Fprintln(w, "{}")
		case fmt.Sprintf("/repos/%v/%v/pulls/%v/comments", expectedOwner, expectedRepo, expectedPR):
			if r.Method == "GET" {
				// list comments - respond with empty array
				fmt.Fprintln(w, "[]")
				break
			}
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
		installationID = 2
		accountID      = 3
		senderID       = 4
	)

	_ = memDB.AddGHInstallation(installationID, accountID, senderID)
	memDB.EnableGHInstallation(installationID)

	memDB.Tools = []db.Tool{
		{Name: "Name", Path: "tool", Args: "-flag %BASE_BRANCH% ./..."},
	}

	cfg := AnalyseConfig{
		eventType:       analyser.EventTypePullRequest,
		installationID:  installationID,
		statusesContext: "ci/gopherci/pr",
		statusesURL:     ts.URL + "/status-url",
		baseURL:         "https://github.com/owner/repo.git",
		baseRef:         "base-branch",
		headURL:         "https://github.com/owner/repo.git",
		headRef:         "head-branch",
		goSrcPath:       "github.com/owner/repo",
		owner:           expectedOwner,
		repo:            expectedRepo,
		pr:              expectedPR,
		sha:             expectedCmtSHA,
	}

	err := g.Analyse(cfg)
	switch {
	case err != nil:
		t.Errorf("did not expect error: %v", err)
	case !statePending:
		t.Errorf("did not set status state to pending")
	case !postedComment:
		t.Errorf("did not post comment")
	case !stateSuccess:
		t.Errorf("did not set status state to success")
	case mockAnalyser.goSrcPath != expectedGoSrcPath:
		t.Errorf("goSrcPath have: %q want: %q", mockAnalyser.goSrcPath, expectedGoSrcPath)
	}
}

func TestPullRequestEvent_noInstall(t *testing.T) {
	g, _, _ := setup(t)

	const installationID = 2
	cfg := AnalyseConfig{installationID: installationID}

	err := g.Analyse(cfg)
	if want := errors.New("could not find installation with ID 2"); err.Error() != want.Error() {
		t.Errorf("expected error %q have %q", want, err)
	}
}

func TestAnalyse_disabled(t *testing.T) {
	g, _, memDB := setup(t)

	const installationID = 2

	// Added but not enabled
	_ = memDB.AddGHInstallation(installationID, 3, 4)

	cfg := AnalyseConfig{installationID: installationID}

	err := g.Analyse(cfg)
	if want := errors.New("could not find installation with ID 2"); err.Error() != want.Error() {
		t.Errorf("expected error %q have %q", want, err)
	}
}

func TestValidPRAction(t *testing.T) {
	tests := []struct {
		action string
		want   bool
	}{
		{"invalid", false},
		{"opened", true},
		{"synchronize", true},
		{"reopened", true},
	}

	for _, test := range tests {
		have := validPRAction(test.action)
		if have != test.want {
			t.Errorf("have: %v want: %v test: %#v", have, test.want, test)
		}
	}
}

func TestStripScheme(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"HTTPS://github.com/owner/repo", "github.com/owner/repo"},
		{"https://github.com/owner/repo", "github.com/owner/repo"},
		{"azAZ09+.-://github.com/owner/repo", "github.com/owner/repo"},
	}
	for _, test := range tests {
		have := stripScheme(test.url)
		if have != test.want {
			t.Errorf("have: %v want: %v", have, test.want)
		}
	}
}

func TestStatusDesc(t *testing.T) {
	tests := []struct {
		issues     []analyser.Issue
		suppressed int
		want       string
	}{
		{[]analyser.Issue{{}, {}}, 2, "Found 2 issues (2 comments suppressed)"},
		{[]analyser.Issue{{}, {}}, 1, "Found 2 issues (1 comment suppressed)"},
		{[]analyser.Issue{{}, {}}, 0, "Found 2 issues"},
		{[]analyser.Issue{{}}, 0, "Found 1 issue"},
		{[]analyser.Issue{}, 0, `Found no issues \ʕ◔ϖ◔ʔ/`},
	}

	for _, test := range tests {
		have := statusDesc(test.issues, test.suppressed)
		if have != test.want {
			t.Errorf("have: %v want: %v", have, test.want)
		}
	}
}

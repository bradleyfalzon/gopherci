package github

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1"
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
	"github.com/bradleyfalzon/gopherci/internal/logger"
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

func (a *mockAnalyser) NewExecuter(_ context.Context, goSrcPath string) (analyser.Executer, error) {
	a.goSrcPath = goSrcPath
	return a, nil
}
func (a *mockAnalyser) Execute(_ context.Context, args []string) (out []byte, err error) {
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
func (a *mockAnalyser) Stop(_ context.Context) error { return nil }

const webhookSecret = "ede9aa6b6e04fafd53f7460fb75644302e249177"

func setup(t *testing.T) (*GitHub, *mockAnalyser, *db.MockDB) {
	memDB := db.NewMockDB()
	mockAnalyser := &mockAnalyser{}
	var (
		wg sync.WaitGroup
		c  = make(chan interface{})
	)
	queue := queue.NewMemoryQueue(logger.Testing())
	queue.Wait(context.Background(), &wg, c, func(job interface{}) {})

	// New GitHub
	g, err := New(logger.Testing(), mockAnalyser, memDB, c, 1, integrationKey, webhookSecret, "https://example.com")
	if err != nil {
		t.Fatal("could not initialise GitHub:", err)
	}
	return g, mockAnalyser, memDB
}

func TestCallbackHandler(t *testing.T) {
	tests := []struct {
		url      string
		wantCode int
	}{
		{"https://example.com/callback?target_url=https%3A%2F%2Fexample.com%2Fresults", http.StatusSeeOther}, // success
		{"https://example.com/callback", http.StatusBadRequest},                                              // no target_url
		{"https://example.com/callback?target_url=https%3A%2F%2Fevil.com", http.StatusBadRequest},            // open redirect
		{"https://example.com/callback?target_url=%", http.StatusBadRequest},                                 // cannot parse form
	}
	for _, test := range tests {
		r := httptest.NewRequest("GET", test.url, nil)
		w := httptest.NewRecorder()

		g, _, _ := setup(t)
		g.CallbackHandler(w, r)

		if w.Code != test.wantCode {
			t.Errorf("code have: %v, want: %v", w.Code, test.wantCode)
			t.Log(w.Body.String())
		}
	}
}

func TestWebhookHandler_signatures(t *testing.T) {
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

func TestWebhookHandler(t *testing.T) {
	goodPush := func() *github.PushEvent {
		return &github.PushEvent{
			Installation: &github.Installation{
				ID: github.Int(1),
			},
			Repo: &github.PushEventRepository{
				ID:          github.Int(2),
				StatusesURL: github.String("https://github.com/owner/repo/status/{sha}"),
				CloneURL:    github.String("https://github.com/owner/repo.git"),
				HTMLURL:     github.String("https://github.com/owner/repo"),
				Private:     github.Bool(false),
			},
			After:   github.String("abcdef"),
			Commits: []github.PushEventCommit{{Added: []string{"main.go"}}},
		}
	}

	goodPR := func() *github.PullRequestEvent {
		return &github.PullRequestEvent{
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
						Private: github.Bool(false),
					},
					Ref: github.String("base-branch"),
				},
				Head: &github.PullRequestBranch{
					Repo: &github.Repository{
						CloneURL: github.String("https://github.com/owner/repo.git"),
						Private:  github.Bool(false),
					},
					SHA: github.String("abcdef"),
					Ref: github.String("head-branch"),
				},
			},
			Installation: &github.Installation{
				ID: github.Int(1),
			},
			Repo: &github.Repository{
				Owner: &github.User{
					Login: github.String("owner"),
				},
				Name:    github.String("repo"),
				ID:      github.Int(2),
				Private: github.Bool(false),
			},
		}
	}

	// Known good push
	push := goodPush()

	// Modify .gopherci.yml
	pushCfg := goodPush()
	pushCfg.Commits = []github.PushEventCommit{{Modified: []string{configFilename}}}

	// No go files
	pushNoGo := goodPush()
	pushNoGo.Commits = []github.PushEventCommit{{Added: []string{"main.php"}}}

	// No valid installation
	pushNoInstall := goodPush()
	pushNoInstall.Installation.ID = github.Int(2)

	// Private repo
	pushPrivateRepo := goodPush()
	pushPrivateRepo.Repo.Private = github.Bool(true)

	// Known good PR
	pr := goodPR()

	// Mock API will respond with .gopherci.yml
	prCfg := goodPR()
	prCfg.Number = github.Int(3)

	// Mock API will respond with no go files
	prNoGo := goodPR()
	prNoGo.Number = github.Int(4)

	// No install
	prNoInstall := goodPR()
	prNoInstall.Installation.ID = github.Int(2)

	// Invalid action
	prInvalidAction := goodPR()
	prInvalidAction.Action = github.String("invalid")

	// Private repo
	prPrivateRepoA := goodPR()
	prPrivateRepoA.Repo.Private = github.Bool(true)
	prPrivateRepoB := goodPR()
	prPrivateRepoB.PullRequest.Head.Repo.Private = github.Bool(true)
	prPrivateRepoC := goodPR()
	prPrivateRepoC.PullRequest.Base.Repo.Private = github.Bool(true)

	tests := []struct {
		payload  interface{}
		event    string
		wantMsg  bool
		wantCode int // http status code we want the response to be
	}{
		{push, "push", true, http.StatusOK},
		{pushCfg, "push", true, http.StatusOK},
		{pushNoGo, "push", false, http.StatusOK},
		{pushNoInstall, "push", false, http.StatusOK},
		{pushPrivateRepo, "push", false, http.StatusOK},
		{pr, "pull_request", true, http.StatusOK},
		{prCfg, "pull_request", true, http.StatusOK},
		{prNoGo, "pull_request", false, http.StatusOK},
		{prNoInstall, "pull_request", false, http.StatusOK},
		{prInvalidAction, "pull_request", false, http.StatusOK},
		{prPrivateRepoA, "pull_request", false, http.StatusOK},
		{prPrivateRepoB, "pull_request", false, http.StatusOK},
		{prPrivateRepoC, "pull_request", false, http.StatusOK},
	}

	const (
		installationID = 1
		accountID      = 2
		senderID       = 3
	)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.RequestURI {
		case "/repos/owner/repo/pulls/2": // checkPRAccessible
		case "/repos/owner/repo/pulls/3": // checkPRAccessible
		case "/repos/owner/repo/pulls/4": // checkPRAccessible
		case "/installations/1/access_tokens":
			// respond with any token to installation transport
			fmt.Fprintln(w, "{}")
		case "/repos/owner/repo/pulls/2/files?per_page=100":
			file := github.CommitFile{Filename: github.String("main.go")}
			js, _ := json.Marshal([]*github.CommitFile{&file})
			fmt.Fprintln(w, string(js))
		case "/repos/owner/repo/pulls/3/files?per_page=100":
			file := github.CommitFile{Filename: github.String(configFilename)}
			js, _ := json.Marshal([]*github.CommitFile{&file})
			fmt.Fprintln(w, string(js))
		case "/repos/owner/repo/pulls/4/files?per_page=100":
			file := github.CommitFile{Filename: github.String("main.php")} // non go file
			js, _ := json.Marshal([]*github.CommitFile{&file})
			fmt.Fprintln(w, string(js))
		default:
			t.Fatalf(r.RequestURI)
		}
	}))
	defer ts.Close()

	for i, test := range tests {
		g, _, memDB := setup(t)
		g.baseURL = ts.URL

		// add installation
		_ = memDB.AddGHInstallation(installationID, accountID, senderID)
		memDB.EnableGHInstallation(installationID)

		// make channel
		c := make(chan interface{}, 1) // buffer of 1 so we don't need to block
		g.queuePush = c

		// make response writer
		w := httptest.NewRecorder()

		// make request
		js, _ := json.Marshal(test.payload)
		r, _ := http.NewRequest("POST", "http://example.com", bytes.NewReader(js))
		r.Header.Add("X-GitHub-Event", test.event)

		sig := hmac.New(sha1.New, g.webhookSecret)
		sig.Write(js)
		r.Header.Add("X-Hub-Signature", fmt.Sprintf("sha1=%x", sig.Sum(nil)))

		// send request
		g.WebHookHandler(w, r)

		// check response code
		if w.Code != test.wantCode {
			t.Errorf("have: %v, want: %v, test: %v", w.Code, test.wantCode, i)
		}

		// check channel
		if test.wantMsg {
			// check length of channel so we don't block forever
			if len(c) < 1 {
				t.Errorf("did not receive message on channel for test %v", i)
			} else {
				haveMsg := <-c

				if !reflect.DeepEqual(haveMsg, test.payload) {
					t.Errorf("have: %v, want: %v, test: %v", haveMsg, test.payload, i)
				}
			}
		} else {
			if len(c) > 0 {
				t.Errorf("unexpected message for test %v: %v", i, <-c)
			}
		}
	}
}

func TestCheckPRAction(t *testing.T) {
	tests := []struct {
		action *string
		want   error
	}{
		{nil, &ignoreEvent{}},
		{github.String("invalid"), &ignoreEvent{}},
		{github.String("opened"), nil},
		{github.String("synchronize"), nil},
		{github.String("reopened"), nil},
	}

	for _, test := range tests {
		have := checkPRAction(&github.PullRequestEvent{Action: test.action})
		if reflect.TypeOf(have) != reflect.TypeOf(test.want) {
			t.Errorf("have: %v want: %v test: %#v", have, test.want, test)
		}
	}
}

func TestCheckPushAffectsGo(t *testing.T) {
	tests := []struct {
		commits github.PushEventCommit
		want    bool
	}{
		{github.PushEventCommit{}, false},
		{github.PushEventCommit{Added: []string{"main.php"}}, false},
		{github.PushEventCommit{Added: []string{"main.go"}}, true},
		{github.PushEventCommit{Removed: []string{"main.go"}}, true},
		{github.PushEventCommit{Modified: []string{"main.go"}}, true},
	}

	for _, test := range tests {
		e := &github.PushEvent{
			Commits: []github.PushEventCommit{test.commits},
		}
		have := checkPushAffectsGo(e)
		if have != test.want {
			t.Errorf("have: %v, want: %v", have, test.want)
		}
	}
}

func TestCheckPRAffectsGo(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.RequestURI {
		case "/installations/1/access_tokens":
			// respond with any token to installation transport
			fmt.Fprintln(w, "{}")
		case "/repos/owner/repo/pulls/2/files?per_page=100":
			file := github.CommitFile{Filename: github.String("main.php")} // first page has no go files
			js, _ := json.Marshal([]*github.CommitFile{&file})
			w.Header().Add("Link", `</repos/owner/repo/pulls/2/files/?page=2&per_page=100>; rel="next"`)
			fmt.Fprintln(w, string(js))
		case "/repos/owner/repo/pulls/2/files?page=2&per_page=100":
			file := github.CommitFile{Filename: github.String("main.go")} // second page does
			js, _ := json.Marshal([]*github.CommitFile{&file})
			fmt.Fprintln(w, string(js))
		default:
			t.Fatalf(r.RequestURI)
		}
	}))
	defer ts.Close()

	const installationID = 1

	// Get installation
	g, _, memDB := setup(t)
	g.baseURL = ts.URL
	_ = memDB.AddGHInstallation(installationID, 2, 3)
	memDB.EnableGHInstallation(installationID)
	installation, err := g.NewInstallation(installationID)
	if err != nil {
		t.Fatal("unexpected error:", err)
	}

	have, err := checkPRAffectsGo(context.Background(), installation, "owner", "repo", 2)
	if err != nil {
		t.Fatal("unexpected error:", err)
	}

	if want := true; have != want {
		t.Errorf("have: %v, want: %v", have, want)
	}
}

func TestCheckPRAccessible(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.RequestURI {
		case "/installations/1/access_tokens":
			// respond with any token to installation transport
			fmt.Fprintln(w, "{}")
		case "/repos/owner/repo/pulls/2":
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprintln(w, `{
    "message": "Not Found",
    "documentation_url": "https://developer.github.com/v3"
}`)
		default:
			t.Fatalf(r.RequestURI)
		}
	}))
	defer ts.Close()

	const installationID = 1

	// Get installation
	g, _, memDB := setup(t)
	g.baseURL = ts.URL
	_ = memDB.AddGHInstallation(installationID, 2, 3)
	memDB.EnableGHInstallation(installationID)
	installation, err := g.NewInstallation(installationID)
	if err != nil {
		t.Fatal("unexpected error:", err)
	}

	err = checkPRAccessible(context.Background(), installation, "owner", "repo", 2)
	ierr, ok := err.(*ignoreEvent)
	if !ok {
		t.Errorf("unexpected error type %T want: *ignoreEvent", err)
	}

	if want := ignorePRInaccessible; ierr.reason != want {
		t.Errorf("unexpected error reason %v want: %v", ierr.reason, want)
	}

	if want := "404 Not Found"; ierr.extra != want {
		t.Errorf("unexpected error extra %v want: %v", ierr.extra, want)
	}
}

func TestIntegrationInstallationEvent(t *testing.T) {
	g, _, memDB := setup(t)

	const (
		installationID = 2
		accountID      = 3
		senderID       = 4
	)

	event := &github.InstallationEvent{
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
		cloner: &analyser.PushCloner{
			HeadURL: "https://github.com/owner/repo.git",
			HeadRef: "abcdef",
		},
		refReader: &analyser.FixedRef{
			BaseRef: "abcdef~2",
		},
		installationID:  1,
		repositoryID:    2,
		statusesContext: "ci/gopherci/push",
		statusesURL:     "https://github.com/owner/repo/status/abcdef",
		commitFrom:      "abcdef~2",
		commitTo:        "abcdef",
		commitCount:     2,
		headRef:         "abcdef",
		goSrcPath:       "github.com/owner/repo",
		owner:           "owner",
		repo:            "repo",
		sha:             "abcdef",
	}

	have := PushConfig(goodPush())
	if !reflect.DeepEqual(have, want) {
		t.Errorf("have:\n%+v\nwant:\n%+v", have, want)
	}
}

func goodPush() *github.PushEvent {
	return &github.PushEvent{
		Installation: &github.Installation{
			ID: github.Int(1),
		},
		Repo: &github.PushEventRepository{
			ID: github.Int(2),
			Owner: &github.PushEventRepoOwner{
				Name: github.String("owner"),
			},
			Name:        github.String("repo"),
			StatusesURL: github.String("https://github.com/owner/repo/status/{sha}"),
			CloneURL:    github.String("https://github.com/owner/repo.git"),
			HTMLURL:     github.String("https://github.com/owner/repo"),
		},
		After:   github.String("abcdef"),
		Commits: []github.PushEventCommit{{}, {}},
		Created: github.Bool(false),
	}
}

func TestPushConfig_created(t *testing.T) {
	e := goodPush()
	e.Created = github.Bool(true)

	have := PushConfig(e)
	if want := ""; have.commitFrom != want {
		t.Errorf("have: %q, want: %q", have, want)
	}
}

func TestPullRequestConfig(t *testing.T) {
	want := AnalyseConfig{
		cloner: &analyser.PullRequestCloner{
			HeadURL: "https://github.com/owner/repo.git",
			HeadRef: "head-branch",
			BaseURL: "https://github.com/owner/repo.git",
			BaseRef: "base-branch",
		},
		refReader:       &analyser.MergeBase{},
		installationID:  1,
		repositoryID:    2,
		statusesContext: "ci/gopherci/pr",
		statusesURL:     "https://github.com/owner/repo/status/abcdef",
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
		Repo: &github.Repository{
			ID: github.Int(2),
		},
	}
	have := PullRequestConfig(e)
	if !reflect.DeepEqual(have, want) {
		t.Errorf("have:\n%+v\nwant:\n%+v", have, want)
	}
}

func TestAnalyse(t *testing.T) {
	g, mockAnalyser, memDB := setup(t)

	var (
		postedComment bool
	)

	var (
		expectedOwner     = "owner"
		expectedRepo      = "repo"
		expectedPR        = 3
		expectedGoSrcPath = "github.com/owner/repo"
	)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decoder := json.NewDecoder(r.Body)
		switch r.RequestURI {
		case "/installations/2/access_tokens":
			// respond with any token to installation transport
			fmt.Fprintln(w, "{}")
		case fmt.Sprintf("/repos/%v/%v/pulls/%v/comments", expectedOwner, expectedRepo, expectedPR):
			if r.Method == "GET" {
				// list comments - respond with empty array
				fmt.Fprintln(w, "[]")
				break
			}
		case fmt.Sprintf("/repos/%v/%v/pulls/%v/reviews", expectedOwner, expectedRepo, expectedPR):
			// Integration test handles the details, we just want to ensure a
			// review was posted.
			var have github.PullRequestReviewRequest
			err := decoder.Decode(&have)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				break
			}
			if want := 1; len(have.Comments) != want {
				t.Errorf("have review comments count: %v, want: %v", len(have.Comments), want)
				break
			}
			postedComment = true
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
		cloner:          &analyser.PushCloner{},
		refReader:       &analyser.FixedRef{BaseRef: "base-branch"},
		installationID:  installationID,
		statusesContext: "ci/gopherci/pr",
		statusesURL:     ts.URL + "/status-url",
		headRef:         "head-branch",
		goSrcPath:       "github.com/owner/repo",
		owner:           expectedOwner,
		repo:            expectedRepo,
		pr:              expectedPR,
		sha:             "abc123",
	}

	err := g.Analyse(cfg)
	switch {
	case err != nil:
		t.Errorf("did not expect error: %v", err)
	case !postedComment:
		t.Errorf("did not post comment")
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

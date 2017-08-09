package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/bradleyfalzon/gopherci/internal/db"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-github/github"
)

func TestPRCommentReporter_filterIssues(t *testing.T) {
	var (
		expectedOwner   = "owner"
		expectedRepo    = "repo"
		expectedPR      = 2
		expectedCmtBody = "body"
		expectedCmtPath = "path.go"
		expectedCmtPos  = 4
	)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.RequestURI {
		case fmt.Sprintf("/repos/%v/%v/pulls/%v/comments", expectedOwner, expectedRepo, expectedPR):
			comments := []*github.PullRequestComment{
				{
					Body:     nil, // nil check
					Path:     nil, // nil check
					Position: nil, // nil check
				},
				{
					Body:     github.String(expectedCmtBody),
					Path:     github.String(expectedCmtPath),
					Position: github.Int(expectedCmtPos),
				},
				{
					Body:     github.String(expectedCmtBody),
					Path:     github.String(expectedCmtPath),
					Position: github.Int(expectedCmtPos + 2),
				},
				{
					// Duplicate comment
					Body:     github.String(expectedCmtBody),
					Path:     github.String(expectedCmtPath),
					Position: github.Int(expectedCmtPos + 2),
				},
			}
			json, _ := json.Marshal(comments)
			fmt.Fprint(w, string(json))
		}
	}))
	defer ts.Close()

	r := NewPRCommentReporter(github.NewClient(nil), expectedOwner, expectedRepo, expectedPR, "")
	r.client.BaseURL, _ = url.Parse(ts.URL)

	var issues = []db.Issue{
		{Path: expectedCmtPath, HunkPos: expectedCmtPos, Issue: expectedCmtBody},     // remove
		{Path: expectedCmtPath, HunkPos: expectedCmtPos + 1, Issue: expectedCmtBody}, // keep
		{Path: expectedCmtPath, HunkPos: expectedCmtPos + 2, Issue: expectedCmtBody}, // remove
	}

	filtered, err := r.filterIssues(context.Background(), issues)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if want := 1; len(filtered) != want {
		t.Errorf("filtered comment count %v does not match %v", len(filtered), want)
	}
}

func TestPRCommentReporter_report(t *testing.T) {
	var (
		expectedOwner   = "owner"
		expectedRepo    = "repo"
		expectedPR      = 2
		expectedCmtBody = "body"
		expectedCmtPath = "path"
		expectedCmtPos  = 4
		expectedCmtSHA  = "abc123"
	)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decoder := json.NewDecoder(r.Body)
		switch r.RequestURI {
		case fmt.Sprintf("/repos/%v/%v/pulls/%v/comments", expectedOwner, expectedRepo, expectedPR):
			if strings.ToLower(r.Method) == "get" {
				// Call to ListComments
				comments := []*github.PullRequestComment{}
				json, _ := json.Marshal(comments)
				fmt.Fprint(w, string(json))
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
			}
		default:
			t.Logf(r.RequestURI)
		}
	}))
	defer ts.Close()

	r := NewPRCommentReporter(github.NewClient(nil), expectedOwner, expectedRepo, expectedPR, expectedCmtSHA)
	r.client.BaseURL, _ = url.Parse(ts.URL)

	var issues = []db.Issue{{Path: expectedCmtPath, HunkPos: expectedCmtPos, Issue: expectedCmtBody}}

	err := r.Report(context.Background(), issues)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStatusAPIReporter_SetStatus(t *testing.T) {
	type status struct {
		State       string `json:"state,omitempty"`
		TargetURL   string `json:"target_url,omitempty"`
		Description string `json:"description,omitempty"`
		Context     string `json:"context,omitempty"`
	}
	var have status

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decoder := json.NewDecoder(r.Body)
		switch r.RequestURI {
		case "/status-url":
			err := decoder.Decode(&have)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		default:
			t.Logf(r.RequestURI)
		}
	}))
	defer ts.Close()
	statusURL := ts.URL + "/status-url"

	var want = status{
		State:       string(StatusStatePending),
		TargetURL:   "https://example.com",
		Description: "description",
		Context:     "context",
	}

	r := NewStatusAPIReporter(github.NewClient(nil), statusURL, want.Context, want.TargetURL)
	r.SetStatus(context.Background(), StatusStatePending, want.Description)

	if diff := cmp.Diff(have, want); diff != "" {
		t.Errorf("unexpected status (-have +want)\n%s", diff)
	}
}

func TestStatusAPIReporter_statusDesc(t *testing.T) {
	tests := []struct {
		issues     []db.Issue
		suppressed int
		want       string
	}{
		{[]db.Issue{{}, {}}, 2, "Found 2 issues (2 comments suppressed)"},
		{[]db.Issue{{}, {}}, 1, "Found 2 issues (1 comment suppressed)"},
		{[]db.Issue{{}, {}}, 0, "Found 2 issues"},
		{[]db.Issue{{}}, 0, "Found 1 issue"},
		{[]db.Issue{}, 0, `Found no issues \ʕ◔ϖ◔ʔ/`},
	}

	r := StatusAPIReporter{}

	for _, test := range tests {
		have := r.statusDesc(test.issues, test.suppressed)
		if have != test.want {
			t.Errorf("have: %v want: %v", have, test.want)
		}
	}
}

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
	"github.com/bradleyfalzon/gopherci/internal/logger"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-github/github"
)

func TestDedupePRIssues(t *testing.T) {
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

	client := github.NewClient(nil)
	client.BaseURL, _ = url.Parse(ts.URL)

	var issues = []db.Issue{
		{Path: expectedCmtPath, HunkPos: expectedCmtPos, Issue: expectedCmtBody},     // remove
		{Path: expectedCmtPath, HunkPos: expectedCmtPos + 1, Issue: expectedCmtBody}, // keep
		{Path: expectedCmtPath, HunkPos: expectedCmtPos + 2, Issue: expectedCmtBody}, // remove
	}

	filtered, err := dedupePRIssues(context.Background(), client, expectedOwner, expectedRepo, expectedPR, issues)
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

	r := NewStatusAPIReporter(logger.Testing(), github.NewClient(nil), statusURL, want.Context, want.TargetURL)
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

func TestCommitCommentReporter_report(t *testing.T) {
	var tests = []struct {
		issues    []db.Issue
		wantBody  string
		wantCount int // number of comments wanted
	}{
		{
			issues: []db.Issue{
				{Issue: "some issue"},
			},
			wantBody:  "GopherCI found **1** issue in the last **2** commits, see: https://example.com",
			wantCount: 1,
		},
		{
			issues: []db.Issue{
				{Issue: "some issue 1"},
				{Issue: "some issue 2"},
			},
			wantBody:  "GopherCI found **2** issues in the last **2** commits, see: https://example.com",
			wantCount: 1,
		},
		{
			issues:    []db.Issue{},
			wantCount: 0,
		},
	}

	for _, test := range tests {
		var (
			expectedOwner  = "owner"
			expectedRepo   = "repo"
			expectedCmtSHA = "abc123"
			commentCount   = 0
		)

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			decoder := json.NewDecoder(r.Body)
			switch r.RequestURI {
			case fmt.Sprintf("/repos/%v/%v/commits/%v/comments", expectedOwner, expectedRepo, expectedCmtSHA):
				commentCount++
				if test.wantCount == 0 {
					// we're not wanting any comments, just increment commentCount
					// and don't check the comment itself
					return
				}
				expected := github.PullRequestComment{
					Body: github.String(test.wantBody),
				}
				var comment github.PullRequestComment
				err := decoder.Decode(&comment)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if diff := cmp.Diff(expected, comment); diff != "" {
					t.Fatalf("expected cmt: (-have +want)\n%s", diff)
				}
			default:
				t.Logf(r.RequestURI)
			}
		}))
		defer ts.Close()

		r := NewCommitCommentReporter(github.NewClient(nil), expectedOwner, expectedRepo, expectedCmtSHA, 2, "https://example.com")
		r.client.BaseURL, _ = url.Parse(ts.URL)

		err := r.Report(context.Background(), test.issues)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if want := test.wantCount; commentCount != want {
			t.Errorf("commentCount want: %v, have: %v", want, commentCount)
		}
	}
}

func TestInlineCommitCommentReporter_report(t *testing.T) {
	var (
		expectedOwner   = "owner"
		expectedRepo    = "repo"
		expectedCmtBody = "body"
		expectedCmtPath = "path"
		expectedCmtPos  = 4
		expectedSHA     = "abc123"
	)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decoder := json.NewDecoder(r.Body)
		switch r.RequestURI {
		case fmt.Sprintf("/repos/%v/%v/commits/%v/comments", expectedOwner, expectedRepo, expectedSHA):
			expected := github.RepositoryComment{
				Body:     github.String(expectedCmtBody),
				Path:     github.String(expectedCmtPath),
				Position: github.Int(expectedCmtPos),
			}
			var comment github.RepositoryComment
			err := decoder.Decode(&comment)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if diff := cmp.Diff(expected, comment); diff != "" {
				t.Fatalf("expected cmt: (-have +want)\n%s", diff)
			}
		default:
			t.Logf(r.RequestURI)
		}
	}))
	defer ts.Close()

	r := NewInlineCommitCommentReporter(github.NewClient(nil), expectedOwner, expectedRepo, expectedSHA)
	r.client.BaseURL, _ = url.Parse(ts.URL)

	var issues = []db.Issue{{Path: expectedCmtPath, HunkPos: expectedCmtPos, Issue: expectedCmtBody}}

	err := r.Report(context.Background(), issues)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPRReviewReporter_report(t *testing.T) {
	var (
		owner = "owner"
		repo  = "repo"
		pr    = 2
		sha   = "abc123"
	)

	var tests = map[string]struct {
		issues []db.Issue
		want   github.PullRequestReviewRequest
	}{
		"noissues": {
			issues: nil,
			want: github.PullRequestReviewRequest{
				Event:    github.String("APPROVE"),
				CommitID: github.String(sha),
				Comments: nil,
			},
		},
		"issues": {
			issues: []db.Issue{
				{Issue: "body", Path: "path.go", HunkPos: 2},
			},
			want: github.PullRequestReviewRequest{
				Event:    github.String("COMMENT"),
				CommitID: github.String(sha),
				Comments: []*github.DraftReviewComment{
					{
						Body:     github.String("body"),
						Path:     github.String("path.go"),
						Position: github.Int(2),
					},
				},
			},
		},
	}

	for desc, test := range tests {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			decoder := json.NewDecoder(r.Body)
			switch r.RequestURI {
			case fmt.Sprintf("/repos/%v/%v/pulls/%v/comments", owner, repo, pr):
				// Call to ListComments
				fmt.Fprintln(w, "[]")
			case fmt.Sprintf("/repos/%v/%v/pulls/%v/reviews", owner, repo, pr):
				var have github.PullRequestReviewRequest
				err := decoder.Decode(&have)
				if err != nil {
					t.Errorf("%v: unexpected error: %v", desc, err)
				}
				if diff := cmp.Diff(have, test.want); diff != "" {
					t.Errorf("%v: expected review (-have +want)%s", desc, diff)
				}
			default:
				t.Logf(r.RequestURI)
			}
		}))
		defer ts.Close()

		r := NewPRReviewReporter(github.NewClient(nil), owner, repo, pr, sha)
		r.client.BaseURL, _ = url.Parse(ts.URL)

		err := r.Report(context.Background(), test.issues)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
}

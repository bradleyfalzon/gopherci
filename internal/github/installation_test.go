package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"

	"github.com/bradleyfalzon/gopherci/internal/analyser"
	"github.com/google/go-github/github"
)

func TestFilterIssues_maxIssueComments(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer ts.Close()

	i := Installation{client: github.NewClient(nil)}
	i.client.BaseURL, _ = url.Parse(ts.URL)

	// Number of issues to suppress
	suppress := 1

	// Add more issues than maxIssueComments
	var issues []analyser.Issue
	for n := 0; n < maxIssueComments+suppress; n++ {
		issues = append(issues, analyser.Issue{File: "file.go", HunkPos: n, Issue: "body"})
	}

	suppressed, filtered, err := i.FilterIssues(context.Background(), "owner", "repo", 2, issues)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Ensure we don't send more comments than maxIssueComments
	if len(filtered) > maxIssueComments {
		t.Errorf("filtered comment count %v is greater than maxIssueComments %v", len(filtered), maxIssueComments)
	}

	if suppressed != suppress {
		t.Errorf("suppressed have %v want %v", suppressed, suppress)
	}
}

func TestFilterIssues_deduplicate(t *testing.T) {
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
			comments := []*github.PullRequestComment{{
				Body:     github.String(expectedCmtBody),
				Path:     github.String(expectedCmtPath),
				Position: github.Int(expectedCmtPos),
			}}
			json, _ := json.Marshal(comments)
			fmt.Fprint(w, string(json))
		}
	}))
	defer ts.Close()

	i := Installation{client: github.NewClient(nil)}
	i.client.BaseURL, _ = url.Parse(ts.URL)

	var issues = []analyser.Issue{{File: expectedCmtPath, HunkPos: expectedCmtPos, Issue: expectedCmtBody}}

	_, filtered, err := i.FilterIssues(context.Background(), expectedOwner, expectedRepo, expectedPR, issues)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if want := 0; len(filtered) != want {
		t.Errorf("filtered comment count %v does not match %v", len(filtered), want)
	}
}

func TestWriteIssues(t *testing.T) {
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

	i := Installation{client: github.NewClient(nil)}
	i.client.BaseURL, _ = url.Parse(ts.URL)

	var issues = []analyser.Issue{{File: expectedCmtPath, HunkPos: expectedCmtPos, Issue: expectedCmtBody}}

	err := i.WriteIssues(context.Background(), expectedOwner, expectedRepo, expectedPR, expectedCmtSHA, issues)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

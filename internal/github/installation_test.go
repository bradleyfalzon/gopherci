package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"

	"github.com/bradleyfalzon/gopherci/internal/db"
	"github.com/google/go-github/github"
)

func TestInstallation_isEnabled(t *testing.T) {
	var i *Installation
	if want := false; i.IsEnabled() != want {
		t.Errorf("want: %v, have: %v", want, i.IsEnabled())
	}
	i = &Installation{}
	if want := true; i.IsEnabled() != want {
		t.Errorf("want: %v, have: %v", want, i.IsEnabled())
	}
}

func TestFilterIssues_maxIssueComments(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer ts.Close()

	i := Installation{client: github.NewClient(nil)}
	i.client.BaseURL, _ = url.Parse(ts.URL)

	// Number of issues to suppress
	suppress := 1

	// Add more issues than maxIssueComments
	var issues []db.Issue
	for n := 0; n < maxIssueComments+suppress; n++ {
		issues = append(issues, db.Issue{Path: "file.go", HunkPos: n, Issue: "body"})
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

	i := Installation{client: github.NewClient(nil)}
	i.client.BaseURL, _ = url.Parse(ts.URL)

	var issues = []db.Issue{
		{Path: expectedCmtPath, HunkPos: expectedCmtPos, Issue: expectedCmtBody},     // remove
		{Path: expectedCmtPath, HunkPos: expectedCmtPos + 1, Issue: expectedCmtBody}, // keep
		{Path: expectedCmtPath, HunkPos: expectedCmtPos + 2, Issue: expectedCmtBody}, // remove
	}

	_, filtered, err := i.FilterIssues(context.Background(), expectedOwner, expectedRepo, expectedPR, issues)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if want := 1; len(filtered) != want {
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

	var issues = []db.Issue{{Path: expectedCmtPath, HunkPos: expectedCmtPos, Issue: expectedCmtBody}}

	err := i.WriteIssues(context.Background(), expectedOwner, expectedRepo, expectedPR, expectedCmtSHA, issues)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInstallation_diff(t *testing.T) {
	var (
		wantDiff = []byte("diff")
		api      []byte
	)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.RequestURI {
		case "/repositories/11/pulls/10":
			// API response for pull requests
			w.Write(api)
		case "/repositories/11/compare/zzzct":
			// API response for first pushes
			w.Write(api)
		case "/repositories/11/compare/zzzct~3...zzzct":
			// API response for pushes
			w.Write(api)
		case "/diff.diff":
			w.Write(wantDiff)
		default:
			t.Logf(r.RequestURI)
		}
	}))
	defer ts.Close()

	api = []byte(fmt.Sprintf(`{"diff_url": "%v/diff.diff"}`, ts.URL))
	i := Installation{client: github.NewClient(nil)}
	i.client.BaseURL, _ = url.Parse(ts.URL)

	tests := []struct {
		commitFrom string
		commitTo   string
		requestNum int
	}{
		{"zzzct~3", "zzzct", 0},
		{"", "", 10},
	}

	for _, test := range tests {
		body, err := i.Diff(context.Background(), 11, test.commitFrom, test.commitTo, test.requestNum)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		haveDiff, err := ioutil.ReadAll(body)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !reflect.DeepEqual(haveDiff, wantDiff) {
			t.Errorf("diff have: %s, want: %s", haveDiff, wantDiff)
		}
	}
}

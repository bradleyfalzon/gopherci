package github

import (
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

func TestWriteIssues(t *testing.T) {
	var (
		expectedOwner   = "owner"
		expectedRepo    = "repo"
		expectedPR      = 2
		expectedCmtBody = "body"
		expectedCmtPath = "path"
		expectedCmtPos  = 4
		expectedCmtSHA  = "abc123"
		commentCount    = 0
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
			} else {
				commentCount++
			}
		default:
			t.Logf(r.RequestURI)
		}
	}))
	defer ts.Close()

	i := Installation{client: github.NewClient(nil)}
	i.client.BaseURL, _ = url.Parse(ts.URL)

	// Number of issues to suppress
	suppress := 1

	// Add more issues than maxIssueComments
	var issues []analyser.Issue
	for n := 0; n < maxIssueComments+suppress; n++ {
		issues = append(issues, analyser.Issue{File: expectedCmtPath, HunkPos: expectedCmtPos, Issue: expectedCmtBody})
	}

	suppressed, err := i.WriteIssues(expectedOwner, expectedRepo, expectedPR, expectedCmtSHA, issues)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Ensure we don't send more comments than maxIssueComments
	if commentCount != maxIssueComments {
		t.Errorf("commentCount %v does not match maxIssueComments %v", commentCount, maxIssueComments)
	}

	if suppressed != suppress {
		t.Errorf("suppressed have %v want %v", suppressed, suppress)
	}
}

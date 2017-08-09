package analyser

import (
	"testing"

	"github.com/bradleyfalzon/gopherci/internal/db"
)

func TestSuppress(t *testing.T) {
	tests := []struct {
		suppress  int // number of issues to suppress
		genIssues int // number of issues to generate
	}{
		{suppress: 1, genIssues: MaxIssueComments + 1},
		{suppress: 0, genIssues: MaxIssueComments},
		{suppress: 0, genIssues: MaxIssueComments - 1},
	}

	for _, test := range tests {
		// Number of issues to suppress
		//suppress := 1

		// Add more issues than maxIssueComments
		var issues []db.Issue
		for n := 0; n < test.genIssues; n++ {
			//for n := 0; n < MaxIssueComments+suppress; n++ {
			issues = append(issues, db.Issue{Path: "file.go", HunkPos: n, Issue: "body"})
		}

		suppressed, filtered := Suppress(issues, MaxIssueComments)

		// Ensure we don't send more comments than maxIssueComments
		if len(filtered) > MaxIssueComments {
			t.Errorf("filtered comment count %v is greater than MaxIssueComments %v", len(filtered), MaxIssueComments)
		}

		if suppressed != test.suppress {
			t.Errorf("suppressed have %v want %v", suppressed, test.suppress)
		}
	}
}

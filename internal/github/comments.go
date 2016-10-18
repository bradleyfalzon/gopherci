package github

import "github.com/bradleyfalzon/gopherci/internal/analyser"

func (g *GitHub) WriteIssues(reviewCommentsURL string, issues []analyser.Issue) {

	// TODO make this idempotent, so don't post the same issue twice
	// which may occur when we support additional commits to a PR (synchronize
	// api event)

	for _, issue := range issues {
		_ = issue

	}

}

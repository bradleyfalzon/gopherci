package analyser

import "github.com/bradleyfalzon/gopherci/internal/db"

// MockAnalyser is an analyser that does nothing, used for testing.
type MockAnalyser struct {
	Tools []db.Tool
	RepoURL,
	Branch,
	DiffURL string
	Issues []Issue
	Err    error
}

// Ensure NOP implements Analyser
var _ Analyser = (*MockAnalyser)(nil)

// Analyse implements Analyser interface
func (m *MockAnalyser) Analyse(tools []db.Tool, repoURL, branch, diffURL string) ([]Issue, error) {
	m.Tools = tools
	m.RepoURL = repoURL
	m.Branch = branch
	m.DiffURL = diffURL
	return m.Issues, m.Err
}

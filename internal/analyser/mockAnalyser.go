package analyser

// MockAnalyser is an analyser that does nothing, used for testing.
type MockAnalyser struct {
	RepoURL,
	Branch,
	DiffURL string
	Issues []Issue
	Err    error
}

// Ensure NOP implements Analyser
var _ Analyser = (*MockAnalyser)(nil)

// Analyse implements Analyser interface
func (m *MockAnalyser) Analyse(repoURL, branch, diffURL string) ([]Issue, error) {
	m.RepoURL = repoURL
	m.Branch = branch
	m.DiffURL = diffURL
	return m.Issues, m.Err
}

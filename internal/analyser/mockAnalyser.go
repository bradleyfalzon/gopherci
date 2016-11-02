package analyser

import "github.com/bradleyfalzon/gopherci/internal/db"

// MockAnalyser is an analyser that does nothing, used for testing.
type MockAnalyser struct {
	Tools  []db.Tool
	Config Config
	Issues []Issue
	Err    error
}

// Ensure NOP implements Analyser
var _ Analyser = (*MockAnalyser)(nil)

// Analyse implements Analyser interface
func (m *MockAnalyser) Analyse(tools []db.Tool, config Config) ([]Issue, error) {
	m.Tools = tools
	m.Config = config
	return m.Issues, m.Err
}

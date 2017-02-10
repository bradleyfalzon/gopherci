package db

import "time"

// DB interface provides access to a persistent database.
type DB interface {
	// AddGHInstallation records a new installation.
	AddGHInstallation(installationID, accountID, senderID int) error
	// RemoveGHInstallation removes an installation.
	RemoveGHInstallation(installationID int) error
	// GetGHInstallation returns an installation for a given installationID, returns
	// nil if no installation was found, or an error occurs.
	GetGHInstallation(installationID int) (*GHInstallation, error)
	// ListTools returns all tools. Returns nil if no tools were found, error will
	// be non-nil if an error occurs.
	ListTools() ([]Tool, error)
	// StartAnalysis records a new analysis.
	StartAnalysis(ghInstallationID, repositoryID int) (analysisID int, err error)
	// FinishAnalysis marks a status as finished.
	FinishAnalysis(analysisID int, status AnalysisStatus, analysis *Analysis) error
}

// AnalysisStatus represents a status in the analysis table.
type AnalysisStatus string

// AnalysisStatus type/enum mappings to the analysis table.
const (
	AnalysisStatusPending AnalysisStatus = "Pending" // Analysis is pending/started (not finished/completed).
	AnalysisStatusFailure AnalysisStatus = "Failure" // Analysis is marked as failed.
	AnalysisStatusSuccess AnalysisStatus = "Success" // Analysis is marked as successful.
	AnalysisStatusError   AnalysisStatus = "Error"   // Analysis failed due to an internal error.
)

// GHInstallation represents a row from the gh_installations table.
type GHInstallation struct {
	ID             int
	InstallationID int
	AccountID      int
	SenderID       int
	enabledAt      time.Time
}

// IsEnabled returns true if the installation is enabled.
func (i GHInstallation) IsEnabled() bool {
	return i.enabledAt.Before(time.Now()) && !i.enabledAt.IsZero()
}

// ToolID is the primary key on the tools table.
type ToolID int

// Tool represents a single tool in the tools table.
type Tool struct {
	ID     ToolID `db:"id"`
	Name   string `db:"name"`
	URL    string `db:"url"`
	Path   string `db:"path"`
	Args   string `db:"args"`
	Regexp string `db:"regexp"`
}

// Analysis represents a single analysis of a repository at a point in time.
type Analysis struct {
	ID               int            `db:"id"`
	GHInstallationID int            `db:"gh_installation_id"`
	RepositoryID     int            `db:"repository_id"`
	Status           AnalysisStatus `db:"status"`

	// When an analysis is finished
	CloneDuration time.Duration `db:"clone_duration"` // CloneDuration is the wall clock time taken to run clone.
	DepsDuration  time.Duration `db:"deps_duration"`  // DepsDuration is the wall clock time taken to fetch dependencies.
	TotalDuration time.Duration `db:"total_duration"` // TotalDuration is the wall clock time taken for the entire analysis.
	Tools         map[ToolID]AnalysisTool
}

// NewAnalysis returns a ready to use analysis.
func NewAnalysis() *Analysis {
	return &Analysis{
		Tools: make(map[ToolID]AnalysisTool),
	}
}

// Issues returns all the issues by each tool as a slice.
func (a *Analysis) Issues() []Issue {
	var issues []Issue
	for _, tool := range a.Tools {
		issues = append(issues, tool.Issues...)
	}
	return issues
}

// AnalysisTool contains the timing and result of an individual tool's analysis.
type AnalysisTool struct {
	Duration time.Duration // Duration is the wall clock time taken to run the tool.
	Issues   []Issue       // Issues maybe nil if no issues found.
}

// Issue contains file, position and string describing a single issue.
type Issue struct {
	// Path is the relative path name of the file.
	Path string
	// Line is the line number of the file.
	Line int
	// HunkPos is the position relative to the files first hunk.
	HunkPos int
	// Issue is the issue.
	Issue string // maybe this should be issue
}

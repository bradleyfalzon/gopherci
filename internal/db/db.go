package db

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"time"
)

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
	StartAnalysis(ghInstallationID, repositoryID int) (*Analysis, error)
	// FinishAnalysis marks a status as finished.
	FinishAnalysis(analysisID int, status AnalysisStatus, analysis *Analysis) error
	// GetAnalysis returns an analysis for a given analysisID, returns nil if no
	// analysis was found, or an error occurs.
	GetAnalysis(analysisID int) (*Analysis, error)
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

var errUnknownAnalysis = errors.New("unknown analysis status")

// Scan implements the sql.Scanner interface.
func (s *AnalysisStatus) Scan(value interface{}) error {
	if value == nil {
		*s = AnalysisStatusPending
		return nil
	}
	switch string(value.([]uint8)) {
	case "Pending":
		*s = AnalysisStatusPending
	case "Failure":
		*s = AnalysisStatusFailure
	case "Success":
		*s = AnalysisStatusSuccess
	case "Error":
		*s = AnalysisStatusError
	default:
		return errUnknownAnalysis
	}
	return nil
}

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

// Duration is similar to a time.Duration but with extra methods to better
// handle mysql DB type TIME(3).
type Duration int64

// Scan implements the sql.Scanner interface.
func (d *Duration) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	t, err := time.Parse("15:04:05.999999999", string(value.([]uint8)))
	if err != nil {
		return err
	}
	*d = Duration(t.AddDate(1970, 0, 0).UnixNano())
	return nil
}

// Value implements the driver.Valuer interface.
func (d Duration) Value() (driver.Value, error) {
	return float64(d) / float64(time.Second), nil
}

// String implements the fmt.Stringer interface.
func (d Duration) String() string {
	return time.Duration(d).String()
}

// Analysis represents a single analysis of a repository at a point in time.
type Analysis struct {
	ID               int            `db:"id"`
	GHInstallationID int            `db:"gh_installation_id"`
	RepositoryID     int            `db:"repository_id"`
	Status           AnalysisStatus `db:"status"`
	CreatedAt        time.Time      `db:"created_at"`

	// When an analysis is finished
	CloneDuration Duration `db:"clone_duration"` // CloneDuration is the wall clock time taken to run clone.
	DepsDuration  Duration `db:"deps_duration"`  // DepsDuration is the wall clock time taken to fetch dependencies.
	TotalDuration Duration `db:"total_duration"` // TotalDuration is the wall clock time taken for the entire analysis.
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

// HTMLURL returns the URL to view the analysis.
func (a *Analysis) HTMLURL(prefix string) string {
	return fmt.Sprintf("%s/analysis/%d", prefix, a.ID)
}

// AnalysisTool contains the timing and result of an individual tool's analysis.
type AnalysisTool struct {
	Tool     *Tool    // Tool is the tool.
	ToolID   ToolID   // ToolID is the ID of the tool.
	Duration Duration // Duration is the wall clock time taken to run the tool.
	Issues   []Issue  // Issues maybe nil if no issues found.
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

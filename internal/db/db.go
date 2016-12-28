package db

import "time"

// DB interface provides access to a persistent database.
type DB interface {
	// AddGHInstallation records a new installation.
	AddGHInstallation(installationID int) error
	// RemoveGHInstallation removes an installation.
	RemoveGHInstallation(accountID int) error
	// GetGHInstallation returns an installation for a given installationID, returns
	// nil if no installation was found, or an error occurs.
	GetGHInstallation(accountID int) (*GHInstallation, error)
	// ListTools returns all tools. Returns nil if no tools were found, error will
	// be non-nil if an error occurs.
	ListTools() ([]Tool, error)
}

type GHInstallation struct {
	InstallationID int
	enabledAt      time.Time
}

// IsEnabled returns true if the installation is enabled.
func (i GHInstallation) IsEnabled() bool {
	return i.enabledAt.Before(time.Now()) && !i.enabledAt.IsZero()
}

// Tool represents a single tool
type Tool struct {
	ID     int    `db:"id"`
	Name   string `db:"name"`
	URL    string `db:"url"`
	Path   string `db:"path"`
	Args   string `db:"args"`
	Regexp string `db:"regexp"`
}

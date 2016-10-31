package db

// DB interface provides access to a persistent database.
type DB interface {
	// GHAddInstallation assigns an installationID to an accountID.
	AddGHInstallation(installationID, accountID int) error
	// GHRemoveInstallation removes an installation from an accountID.
	RemoveGHInstallation(accountID int) error
	// GHFindInstallation returns an installation for a given accountID, returns
	// nil if no installation was found, or an error occurs.
	FindGHInstallation(accountID int) (*GHInstallation, error)
	// ListTools returns all tools. Returns nil if no tools were found, error will
	// be non-nil if an error occurs.
	ListTools() ([]Tool, error)
}

type GHInstallation struct {
	ID             int `db:"id"`
	InstallationID int `db:"installation_id"`
	AccountID      int `db:"account_id"`
}

// Tool represents a single tool
type Tool struct {
	ID         int    `db:"id"`
	Name       string `db:"name"`
	URL        string `db:"url"`
	Path       string `db:"path"`
	Args       string `db:"args"`
	ArgBaseSHA string `db:"argBaseSha"`
	Regexp     string `db:"regexp"`
}

//
//type ToolArg struct {

//}

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
}

type GHInstallation struct {
	ID             int `db:"id"`
	InstallationID int `db:"installation_id"`
	AccountID      int `db:"account_id"`
}

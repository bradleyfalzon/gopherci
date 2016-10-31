package db

import (
	"database/sql"

	"github.com/jmoiron/sqlx"
)

// SQLDB is a sql database repository implementing the DB interface
type SQLDB struct {
	sqlx *sqlx.DB
}

// Ensure SQLDB implements DB
var _ DB = (*SQLDB)(nil)

// NewSQLDB returns an SQLDB
func NewSQLDB(sqlDB *sql.DB, driverName string) (*SQLDB, error) {
	db := &SQLDB{
		sqlx: sqlx.NewDb(sqlDB, driverName),
	}
	if err := db.sqlx.Ping(); err != nil {
		return nil, err
	}
	return db, nil
}

// AddGHInstallation implements DB interface
func (db *SQLDB) AddGHInstallation(installationID, accountID int) error {
	_, err := db.sqlx.Exec("INSERT INTO gh_installations (installation_id, account_id) VALUES (?, ?)", installationID, accountID)
	return err
}

// RemoveGHInstallation implements DB interface
func (db *SQLDB) RemoveGHInstallation(accountID int) error {
	_, err := db.sqlx.Exec("DELETE FROM gh_installations WHERE account_id = ?", accountID)
	return err
}

// FindGHInstallation implements DB interface
func (db *SQLDB) FindGHInstallation(accountID int) (*GHInstallation, error) {
	var installation GHInstallation
	err := db.sqlx.Get(&installation, "SELECT id, installation_id, account_id FROM gh_installations WHERE account_id = ?", accountID)
	return &installation, err
}

func (db *SQLDB) ListTools() ([]Tool, error) {
	// TODO actually read from database
	return []Tool{
		{1, "go vet", "https://golang.org/cmd/vet/", "go", "vet ./...", "", ""},
		{2, "golint", "https://github.com/golang/lint", "golint", "./...", "", ""},
		{3, "apicompat", "https://github.com/bradleyfalzon/apicompat", "apicompat", "./...", "-before", `.*?:(.*?\.go):([0-9]+):()(.*)`},
	}, nil
}

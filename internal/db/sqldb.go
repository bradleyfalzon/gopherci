package db

import (
	"database/sql"

	"github.com/go-sql-driver/mysql"
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
func (db *SQLDB) AddGHInstallation(installationID int) error {
	// INSERT IGNORE so any duplicates are ignored
	_, err := db.sqlx.Exec("INSERT IGNORE INTO gh_installations (installation_id) VALUES (?)", installationID)
	return err
}

// RemoveGHInstallation implements DB interface
func (db *SQLDB) RemoveGHInstallation(installationID int) error {
	_, err := db.sqlx.Exec("DELETE FROM gh_installations WHERE installation_id = ?", installationID)
	return err
}

// GetGHInstallation implements DB interface
func (db *SQLDB) GetGHInstallation(installationID int) (*GHInstallation, error) {
	type ghinstall struct {
		InstallationID int            `db:"installation_id"`
		EnabledAt      mysql.NullTime `db:"enabled_at"`
	}
	var i ghinstall
	err := db.sqlx.Get(&i, "SELECT installation_id, enabled_at FROM gh_installations WHERE installation_id = ?", installationID)
	switch {
	case err == sql.ErrNoRows:
		return nil, nil
	case err != nil:
		return nil, err
	}
	ghi := &GHInstallation{InstallationID: i.InstallationID}
	if i.EnabledAt.Valid {
		ghi.enabledAt = i.EnabledAt.Time
	}
	return ghi, nil
}

func (db *SQLDB) ListTools() ([]Tool, error) {
	var tools []Tool
	err := db.sqlx.Select(&tools, "SELECT name, path, args, `regexp` FROM tools")
	return tools, err
}

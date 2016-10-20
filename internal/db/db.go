package db

import (
	"database/sql"

	"github.com/jmoiron/sqlx"
)

// DB is a sql database repository
type DB struct {
	sqlx *sqlx.DB
}

func NewDB(sqlDB *sql.DB, driverName string) (*DB, error) {
	db := &DB{
		sqlx: sqlx.NewDb(sqlDB, driverName),
	}
	if err := db.sqlx.Ping(); err != nil {
		return nil, err
	}
	return db, nil
}

func (db *DB) GHAddInstallation(installationID, accountID int) error {
	_, err := db.sqlx.Exec(`
INSERT INTO gh_installations (installation_id, account_id)
	 VALUES (?, ?)`,
		installationID, accountID,
	)
	return err
}

func (db *DB) GHRemoveInstallation(installationID, accountID int) error {
	_, err := db.sqlx.Exec("DELETE FROM gh_installations WHERE installation_id = ? AND account_id = ?", installationID, accountID)
	return err
}

type GHInstallation struct {
	ID             int `db:"id"`
	InstallationID int `db:"installation_id"`
	AccountID      int `db:"account_id"`
}

func (db *DB) GHFindInstallation(accountID int) (*GHInstallation, error) {
	var installation GHInstallation
	err := db.sqlx.Get(&installation, "SELECT id, installation_id, account_id FROM gh_installations WHERE account_id = ?", accountID)
	return &installation, err
}

package db

import (
	"database/sql"
	"log"
)

// DB is a sql database repository
type DB struct {
	sql *sql.DB
}

func NewDB(sqlDB *sql.DB) (*DB, error) {
	db := &DB{
		sql: sqlDB,
	}
	if err := db.sql.Ping(); err != nil {
		return nil, err
	}
	return db, nil
}

func (db *DB) GHAddInstallation(installationID, accountID int) error {
	_, err := db.sql.Exec(`
INSERT INTO gh_installations (installation_id, account_id)
	 VALUES (?, ?)`,
		installationID, accountID,
	)
	return err
}

func (db *DB) GHRemoveInstallation(installationID, accountID int) error {
	_, err := db.sql.Exec("DELETE FROM gh_installations WHERE installation_id = ? AND account_id = ?", installationID, accountID)
	return err
}

func (db *DB) GHUserSettings(login string) ([]string, error) {
	log.Printf("looking up user settings for %v", login)
	// SELECT ..... FROM....
	return nil, nil
}

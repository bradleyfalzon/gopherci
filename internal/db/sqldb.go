package db

import (
	"database/sql"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

// SQLDB is a sql database repository implementing the DB interface.
type SQLDB struct {
	sqlx *sqlx.DB
}

// Ensure SQLDB implements DB.
var _ DB = (*SQLDB)(nil)

// NewSQLDB returns an SQLDB.
func NewSQLDB(sqlDB *sql.DB, driverName string) (*SQLDB, error) {
	db := &SQLDB{
		sqlx: sqlx.NewDb(sqlDB, driverName),
	}
	if err := db.sqlx.Ping(); err != nil {
		return nil, err
	}
	return db, nil
}

// AddGHInstallation implements the DB interface.
func (db *SQLDB) AddGHInstallation(installationID, accountID, senderID int) error {
	// INSERT IGNORE so any duplicates are ignored
	_, err := db.sqlx.Exec("INSERT IGNORE INTO gh_installations (installation_id, account_id, sender_id) VALUES (?, ?, ?)",
		installationID, accountID, senderID,
	)
	return err
}

// RemoveGHInstallation implements the DB interface.
func (db *SQLDB) RemoveGHInstallation(installationID int) error {
	_, err := db.sqlx.Exec("DELETE FROM gh_installations WHERE installation_id = ?", installationID)
	return err
}

// GetGHInstallation implements the DB interface.
func (db *SQLDB) GetGHInstallation(installationID int) (*GHInstallation, error) {
	var row struct {
		ID             int            `db:"id"`
		InstallationID int            `db:"installation_id"`
		AccountID      int            `db:"account_id"`
		SenderID       int            `db:"sender_id"`
		EnabledAt      mysql.NullTime `db:"enabled_at"`
	}
	err := db.sqlx.Get(&row, "SELECT id, installation_id, account_id, sender_id, enabled_at FROM gh_installations WHERE installation_id = ?", installationID)
	switch {
	case err == sql.ErrNoRows:
		return nil, nil
	case err != nil:
		return nil, err
	}
	ghi := &GHInstallation{
		ID:             row.ID,
		InstallationID: row.InstallationID,
		AccountID:      row.AccountID,
		SenderID:       row.SenderID,
	}
	if row.EnabledAt.Valid {
		ghi.enabledAt = row.EnabledAt.Time
	}
	return ghi, nil
}

// ListTools implements the DB interface.
func (db *SQLDB) ListTools() ([]Tool, error) {
	var tools []Tool
	err := db.sqlx.Select(&tools, "SELECT id, name, path, args, `regexp` FROM tools")
	return tools, err
}

// StartAnalysis implements the DB interface.
func (db *SQLDB) StartAnalysis(ghInstallationID, repositoryID int) (int, error) {
	result, err := db.sqlx.Exec("INSERT INTO analysis (gh_installation_id, repository_id) VALUES (?, ?)", ghInstallationID, repositoryID)
	if err != nil {
		return 0, err
	}
	analysisID, err := result.LastInsertId()
	return int(analysisID), err
}

// FinishAnalysis implements the DB interface.
func (db *SQLDB) FinishAnalysis(analysisID int, status AnalysisStatus, analysis *Analysis) error {
	if analysis == nil {
		_, err := db.sqlx.Exec("UPDATE analysis SET status = ? WHERE id = ?", string(status), analysisID)
		return err
	}
	_, err := db.sqlx.Exec("UPDATE analysis SET status = ?, clone_duration = ?, deps_duration = ?, total_duration = ? WHERE id = ?",
		string(status), analysis.CloneDuration/time.Second, analysis.DepsDuration/time.Second, analysis.TotalDuration/time.Second, analysisID,
	)
	if err != nil {
		return err
	}
	for toolID, tool := range analysis.Tools {
		toolResult, err := db.sqlx.Exec("INSERT INTO analysis_tool (analysis_id, tool_id, duration) VALUES (?, ?, ?)", analysisID, toolID, tool.Duration/time.Second)
		if err != nil {
			return err
		}

		toolAnalysisID, err := toolResult.LastInsertId()
		if err != nil {
			return err
		}

		for _, issue := range tool.Issues {
			db.sqlx.Exec("INSERT INTO issues (analysis_tool_id, filename, line, hunk_pos, issue) VALUES(?, ?, ?, ?, ?)",
				toolAnalysisID, issue.Path, issue.Line, issue.HunkPos, issue.Issue,
			)
		}

	}
	return nil
}

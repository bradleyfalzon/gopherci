package db

import (
	"database/sql"

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
func (db *SQLDB) StartAnalysis(ghInstallationID, repositoryID int) (*Analysis, error) {
	analysis := NewAnalysis()
	result, err := db.sqlx.Exec("INSERT INTO analysis (gh_installation_id, repository_id) VALUES (?, ?)", ghInstallationID, repositoryID)
	if err != nil {
		return nil, err
	}
	analysisID, err := result.LastInsertId()
	analysis.ID = int(analysisID)
	return analysis, err
}

// FinishAnalysis implements the DB interface.
func (db *SQLDB) FinishAnalysis(analysisID int, status AnalysisStatus, analysis *Analysis) error {
	if analysis == nil {
		_, err := db.sqlx.Exec("UPDATE analysis SET status = ? WHERE id = ?", string(status), analysisID)
		return err
	}
	_, err := db.sqlx.Exec("UPDATE analysis SET status = ?, clone_duration = SEC_TO_TIME(?), deps_duration = SEC_TO_TIME(?), total_duration = SEC_TO_TIME(?) WHERE id = ?",
		string(status), analysis.CloneDuration, analysis.DepsDuration, analysis.TotalDuration, analysisID,
	)
	if err != nil {
		return err
	}

	if analysis.IsPush() {
		_, err = db.sqlx.Exec("UPDATE analysis SET commit_from = ?, commit_to = ? WHERE id = ?", analysis.CommitFrom, analysis.CommitTo, analysisID)
	} else {
		_, err = db.sqlx.Exec("UPDATE analysis SET request_number = ? WHERE id = ?", analysis.RequestNumber, analysisID)
	}
	if err != nil {
		return err
	}

	for toolID, tool := range analysis.Tools {
		toolResult, err := db.sqlx.Exec("INSERT INTO analysis_tool (analysis_id, tool_id, duration) VALUES (?, ?, SEC_TO_TIME(?))", analysisID, toolID, tool.Duration)
		if err != nil {
			return err
		}

		toolAnalysisID, err := toolResult.LastInsertId()
		if err != nil {
			return err
		}

		for _, issue := range tool.Issues {
			_, err := db.sqlx.Exec("INSERT INTO issues (analysis_tool_id, path, line, hunk_pos, issue) VALUES(?, ?, ?, ?, ?)",
				toolAnalysisID, issue.Path, issue.Line, issue.HunkPos, issue.Issue,
			)
			if err != nil {
				return err
			}
		}

	}
	return nil
}

// Foo is an example of staticcheck's incredibly useful SA4006
func (db *SQLDB) Foo() error {
	_, err := db.sqlx.Exec("SELECT 1")
	_, err = db.sqlx.Exec("SELECT 1")
	return err
}

// GetAnalysis implements the DB interface.
func (db *SQLDB) GetAnalysis(analysisID int) (*Analysis, error) {
	analysis := NewAnalysis()

	err := db.sqlx.Get(analysis, `
   SELECT a.id, a.repository_id, IFNULL(a.commit_from, "") commit_from, IFNULL(a.commit_to, "") commit_to,
          IFNULL(a.request_number, 0) request_number, a.status, a.clone_duration, a.deps_duration,
          a.total_duration, a.created_at, IFNULL(ghi.installation_id, 0) installation_id
     FROM analysis a
LEFT JOIN gh_installations ghi ON (a.gh_installation_id = ghi.id)
    WHERE a.id = ?`, analysisID)
	switch {
	case err == sql.ErrNoRows:
		return nil, nil
	case err != nil:
		return nil, err
	}

	var toolIssues []struct {
		ToolID   int            `db:"tool_id"`
		Name     string         `db:"name"`
		URL      string         `db:"url"`
		Duration Duration       `db:"duration"`
		Path     sql.NullString `db:"path"`
		Line     sql.NullInt64  `db:"line"`
		HunkPos  sql.NullInt64  `db:"hunk_pos"`
		Issue    sql.NullString `db:"issue"`
	}

	// get all the tools and issues if they have them
	err = db.sqlx.Select(&toolIssues, `
   SELECT at.tool_id, at.duration, i.path, i.line, i.hunk_pos, i.issue,
		  t.name, t.url
     FROM analysis_tool at
	 JOIN tools t ON (at.tool_id = t.id)
LEFT JOIN issues i ON (i.analysis_tool_id = at.id)
    WHERE at.analysis_id = ?`,
		analysisID,
	)
	if err != nil {
		return nil, err
	}

	for _, issue := range toolIssues {
		toolID := ToolID(issue.ToolID)
		if _, ok := analysis.Tools[toolID]; !ok {
			analysis.Tools[toolID] = AnalysisTool{
				Tool:     &Tool{ID: toolID, Name: issue.Name, URL: issue.URL},
				ToolID:   toolID,
				Duration: issue.Duration,
			}
		}

		if issue.Issue.Valid {
			at := analysis.Tools[toolID]
			at.Issues = append(at.Issues, Issue{
				Path:    issue.Path.String,
				Line:    int(issue.Line.Int64),
				HunkPos: int(issue.HunkPos.Int64),
				Issue:   issue.Issue.String,
			})
			analysis.Tools[toolID] = at
		}
	}

	return analysis, nil
}

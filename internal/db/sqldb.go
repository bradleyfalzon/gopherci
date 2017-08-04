package db

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"
	"unicode"

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

// Cleanup runs background cleanup tasks, such as purging old records.
func (db *SQLDB) Cleanup(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, err := db.sqlx.Exec(`DELETE o FROM outputs o JOIN analysis a ON(o.analysis_id = a.id) WHERE a.created_at < DATE_SUB(NOW(), INTERVAL 30 DAY);`)
			if err != nil {
				log.Println("SQLDB cleanup outputs error:", err)
			}
		}
	}
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
func (db *SQLDB) StartAnalysis(ghInstallationID, repositoryID int, commitFrom, commitTo string, requestNumber int) (*Analysis, error) {
	analysis := NewAnalysis()
	result, err := db.sqlx.Exec("INSERT INTO analysis (gh_installation_id, repository_id) VALUES (?, ?)", ghInstallationID, repositoryID)
	if err != nil {
		return nil, err
	}
	analysisID, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	analysis.ID = int(analysisID)
	analysis.CommitFrom = commitFrom
	analysis.CommitTo = commitTo
	analysis.RequestNumber = requestNumber

	if analysis.IsPush() {
		if analysis.CommitFrom != "" {
			_, err = db.sqlx.Exec("UPDATE analysis SET commit_from = ?, commit_to = ? WHERE id = ?", analysis.CommitFrom, analysis.CommitTo, analysis.ID)
		} else {
			_, err = db.sqlx.Exec("UPDATE analysis SET commit_to = ? WHERE id = ?", analysis.CommitTo, analysis.ID)
		}
	} else {
		_, err = db.sqlx.Exec("UPDATE analysis SET request_number = ? WHERE id = ?", analysis.RequestNumber, analysis.ID)
	}
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
		LineID   sql.NullInt64  `db:"issue_id"`
		Path     sql.NullString `db:"path"`
		Line     sql.NullInt64  `db:"line"`
		HunkPos  sql.NullInt64  `db:"hunk_pos"`
		Issue    sql.NullString `db:"issue"`
	}

	// get all the tools and issues if they have them
	err = db.sqlx.Select(&toolIssues, `
   SELECT at.tool_id, at.duration, i.id issue_id, i.path, i.line, i.hunk_pos, i.issue,
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
				ID:      int(issue.LineID.Int64),
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

// AnalysisOutputs implements the DB interface.
func (db *SQLDB) AnalysisOutputs(analysisID int) ([]Output, error) {
	var tools []Output
	err := db.sqlx.Select(&tools, "SELECT id, analysis_id, arguments, duration, output FROM outputs WHERE analysis_id = ? ORDER BY id ASC", analysisID)
	return tools, err
}

// ExecRecorder implements the DB interface.
func (db *SQLDB) ExecRecorder(analysisID int, executer Executer) Executer {
	return &SQLExecuteWriter{
		analysisID: analysisID,
		executer:   executer,
		db:         db,
	}
}

// WriteExecution writes the results of an execution to the database.
func (db *SQLDB) WriteExecution(analysisID int, args []string, d time.Duration, output []byte) error {
	output = bytes.TrimRightFunc(output, unicode.IsSpace) // remove trailing newlines
	if output == nil {
		output = []byte{} // output column cannot be null
	}

	if len(args) >= 2 && args[0] == "git" && args[1] == "diff" {
		// Never store git diff output, it's used internally for revgrep
		// only, is usually large and can usually be available elsewhere.
		output = []byte(fmt.Sprintf("%d bytes suppressed", len(output)))
	}

	_, err := db.sqlx.Exec("INSERT INTO outputs (analysis_id, arguments, duration, output) VALUES(?, ?, SEC_TO_TIME(?), ?)",
		analysisID, strings.Join(args, " "), Duration(d), trim(output, maxAnalysisOutput),
	)
	return err
}

// maxAnalysisOutput is the approximate maximum number of bytes stored in the
// analysis_output table's output column.
const maxAnalysisOutput = 10240

// trim trims input b to approximately max by keeping the first and last max/2
// bytes. It may be larger due to n bytes suppressed placeholder message.
func trim(b []byte, max int) []byte {
	if len(b) <= max {
		return b
	}

	head := max / 2
	tail := len(b) - max/2
	return []byte(fmt.Sprintf("%s...%d bytes suppressed...%s", b[:head], len(b)-max, b[tail:]))
}

// SQLExecuteWriter wraps an Executer and writes the results of execution to db.
type SQLExecuteWriter struct {
	analysisID int
	executer   Executer
	db         *SQLDB
}

var _ Executer = &SQLExecuteWriter{}

// Executer is the same interface as analyser.Executer, but due to import cycles
// must be redefined here.
type Executer interface {
	Execute(context.Context, []string) ([]byte, error)
	Stop(context.Context) error
}

// Execute implements the Execute interface by running the wrapped executer
// and storing the results in an SQL database.
func (e *SQLExecuteWriter) Execute(ctx context.Context, args []string) ([]byte, error) {
	start := time.Now()
	out, eerr := e.executer.Execute(ctx, args)

	// Write results to DB
	werr := e.db.WriteExecution(e.analysisID, args, time.Since(start), out)
	if werr != nil {
		// execution error may be nil, if execution was successful, but the
		// write to the database was not.
		return out, fmt.Errorf("could not write execution results to db: %v, execution error (may be nil): %v", werr, eerr)
	}
	return out, eerr
}

// Stop implements the Execute interface.
func (e *SQLExecuteWriter) Stop(ctx context.Context) error {
	return e.executer.Stop(ctx)
}

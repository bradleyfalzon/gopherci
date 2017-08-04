package db

import "time"

// MockDB is an in-memory database repository implementing the DB interface
// used for testing
type MockDB struct {
	installations map[int]GHInstallation // installationID -> exists
	err           error
	Tools         []Tool
}

// Ensure MockDB implements DB
var _ DB = (*MockDB)(nil)

// NewMockDB returns an MockDB
func NewMockDB() *MockDB {
	return &MockDB{
		installations: make(map[int]GHInstallation),
	}
}

// ForceError forces MockDB to return err on all methods that return an error.
func (db *MockDB) ForceError(err error) {
	db.err = err
}

// AddGHInstallation implements DB interface
func (db *MockDB) AddGHInstallation(installationID, accountID, senderID int) error {
	db.installations[installationID] = GHInstallation{
		InstallationID: installationID,
		AccountID:      accountID,
		SenderID:       senderID,
	}
	return db.err
}

// RemoveGHInstallation implements DB interface
func (db *MockDB) RemoveGHInstallation(installationID int) error {
	delete(db.installations, installationID)
	return db.err
}

// EnableGHInstallation enables a gh installation
func (db *MockDB) EnableGHInstallation(installationID int) error {
	install := db.installations[installationID]
	install.enabledAt = time.Unix(1, 0)
	db.installations[installationID] = install
	return db.err
}

// GetGHInstallation implements DB interface
func (db *MockDB) GetGHInstallation(installationID int) (*GHInstallation, error) {
	if installation, ok := db.installations[installationID]; ok {
		return &installation, db.err
	}
	return nil, db.err
}

// ListTools implements DB interface
func (db *MockDB) ListTools() ([]Tool, error) {
	return db.Tools, nil
}

// StartAnalysis implements the DB interface.
func (db *MockDB) StartAnalysis(ghInstallationID, repositoryID int, commitFrom, commitTo string, requestNumber int) (*Analysis, error) {
	analysis := NewAnalysis()
	analysis.ID = 99
	analysis.CommitFrom = commitFrom
	analysis.CommitTo = commitTo
	analysis.RequestNumber = requestNumber
	return analysis, nil
}

// FinishAnalysis implements the DB interface.
func (db *MockDB) FinishAnalysis(analysisID int, status AnalysisStatus, analysis *Analysis) error {
	return nil
}

// GetAnalysis implements the DB interface.
func (db *MockDB) GetAnalysis(analysisID int) (*Analysis, error) {
	return nil, nil
}

// ExecRecorder implements the DB interface.
func (db *MockDB) ExecRecorder(analysisID int, executer Executer) Executer {
	return executer
}

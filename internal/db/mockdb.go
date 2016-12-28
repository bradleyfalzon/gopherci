package db

import "time"

// MockDB is an in-memory database repository implementing the DB interface
// used for testing
type MockDB struct {
	installations map[int]bool // installationID -> exists
	err           error
	Tools         []Tool
}

// Ensure MockDB implements DB
var _ DB = (*MockDB)(nil)

// NewMockDB returns an MockDB
func NewMockDB() *MockDB {
	return &MockDB{
		installations: make(map[int]bool),
	}
}

func (db *MockDB) ForceError(err error) {
	db.err = err
}

// AddGHInstallation implements DB interface
func (db *MockDB) AddGHInstallation(installationID int) error {
	db.installations[installationID] = true
	return db.err
}

// RemoveGHInstallation implements DB interface
func (db *MockDB) RemoveGHInstallation(installationID int) error {
	delete(db.installations, installationID)
	return db.err
}

// GetGHInstallation implements DB interface
func (db *MockDB) GetGHInstallation(installationID int) (*GHInstallation, error) {
	if _, ok := db.installations[installationID]; ok {
		return &GHInstallation{InstallationID: installationID, enabledAt: time.Unix(1, 0)}, db.err
	}
	return nil, db.err
}

// ListTools implements DB interface
func (db *MockDB) ListTools() ([]Tool, error) {
	return db.Tools, nil
}

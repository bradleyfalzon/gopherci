package db

// MockDB is an in-memory database repository implementing the DB interface
// used for testing
type MockDB struct {
	installations map[int]int // accountID -> installationID
	err           error
}

// Ensure MockDB implements DB
var _ DB = (*MockDB)(nil)

// NewMockDB returns an MockDB
func NewMockDB() *MockDB {
	return &MockDB{
		installations: make(map[int]int),
	}
}

func (db *MockDB) ForceError(err error) {
	db.err = err
}

// AddGHInstallation implements DB interface
func (db *MockDB) AddGHInstallation(installationID, accountID int) error {
	db.installations[accountID] = installationID
	return db.err
}

// RemoveGHInstallation implements DB interface
func (db *MockDB) RemoveGHInstallation(accountID int) error {
	delete(db.installations, accountID)
	return db.err
}

// FindGHInstallation implements DB interface
func (db *MockDB) FindGHInstallation(accountID int) (*GHInstallation, error) {
	if installationID, ok := db.installations[accountID]; ok {
		return &GHInstallation{AccountID: accountID, InstallationID: installationID}, db.err
	}
	return nil, db.err
}

// ListTools implements DB interface
func (db *MockDB) ListTools() ([]Tool, error) {
	return nil, nil
}

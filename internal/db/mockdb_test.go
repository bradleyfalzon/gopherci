package db

import (
	"reflect"
	"testing"
)

func TestMockDB(t *testing.T) {
	db := NewMockDB()

	const (
		accountID      = 1
		installationID = 2
	)

	err := db.AddGHInstallation(installationID, accountID)
	if err != nil {
		t.Fatal("unexpected error:", err)
	}

	exp := &GHInstallation{
		AccountID:      accountID,
		InstallationID: installationID,
	}

	installation, err := db.FindGHInstallation(accountID)
	if err != nil {
		t.Fatal("unexpected error:", err)
	}

	if !reflect.DeepEqual(exp, installation) {
		t.Fatalf("exp: %#v, got: %#v", exp, installation)
	}

	err = db.RemoveGHInstallation(accountID)
	if err != nil {
		t.Fatal("unexpected error:", err)
	}

	installation, err = db.FindGHInstallation(accountID)
	if err != nil {
		t.Fatal("unexpected error:", err)
	}
	if installation != nil {
		t.Fatal("expected nil, got:", installation)
	}
}

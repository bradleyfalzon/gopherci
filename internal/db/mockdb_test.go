package db

import (
	"reflect"
	"testing"
)

func TestMockDB(t *testing.T) {
	db := NewMockDB()

	const (
		installationID = 2
		accountID      = 3
		senderID       = 4
	)

	err := db.AddGHInstallation(installationID, accountID, senderID)
	if err != nil {
		t.Fatal("unexpected error:", err)
	}

	want := &GHInstallation{
		InstallationID: installationID,
		AccountID:      accountID,
		SenderID:       senderID,
	}

	installation, err := db.GetGHInstallation(installationID)
	if err != nil {
		t.Fatal("unexpected error:", err)
	}

	if !reflect.DeepEqual(installation, want) {
		t.Fatalf("Received incorrect installation\nhave: %#v\nwant: %#v", installation, want)
	}

	err = db.RemoveGHInstallation(installationID)
	if err != nil {
		t.Fatal("unexpected error:", err)
	}

	installation, err = db.GetGHInstallation(installationID)
	if err != nil {
		t.Fatal("unexpected error:", err)
	}
	if installation != nil {
		t.Fatal("expected nil, got:", installation)
	}
}

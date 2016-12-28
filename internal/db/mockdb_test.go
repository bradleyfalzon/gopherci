package db

import (
	"reflect"
	"testing"
	"time"
)

func TestMockDB(t *testing.T) {
	db := NewMockDB()

	const installationID = 2

	err := db.AddGHInstallation(installationID)
	if err != nil {
		t.Fatal("unexpected error:", err)
	}

	exp := &GHInstallation{
		InstallationID: installationID,
		enabledAt:      time.Unix(1, 0),
	}

	installation, err := db.GetGHInstallation(installationID)
	if err != nil {
		t.Fatal("unexpected error:", err)
	}

	if !reflect.DeepEqual(exp, installation) {
		t.Fatalf("exp: %#v, got: %#v", exp, installation)
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

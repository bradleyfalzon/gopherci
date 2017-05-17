package analyser

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/bradleyfalzon/gopherci/internal/db"
)

func TestYAMLConfig_default(t *testing.T) {
	exec := &mockAnalyser{
		ExecuteOut: [][]byte{{}},
		ExecuteErr: []error{&NonZeroError{ExitCode: 1}},
	}

	reader := &YAMLConfig{
		Tools: []db.Tool{{Name: "tool1"}},
	}
	have, err := reader.Read(context.Background(), exec)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	want := RepoConfig{Tools: reader.Tools}

	if !reflect.DeepEqual(have, want) {
		t.Errorf("\nhave: %v\nwant: %v", have, want)
	}
}

func TestYAMLConfig_unknownError(t *testing.T) {
	exec := &mockAnalyser{
		ExecuteOut: [][]byte{{}},
		ExecuteErr: []error{errors.New("unknown error")},
	}

	reader := &YAMLConfig{}
	_, err := reader.Read(context.Background(), exec)
	if err == nil {
		t.Errorf("expected error, have: %v", err)
	}
}

func TestYAMLConfig_unmarshalError(t *testing.T) {
	contents := []byte("\t")
	exec := &mockAnalyser{
		ExecuteOut: [][]byte{contents},
		ExecuteErr: []error{nil},
	}

	reader := &YAMLConfig{}
	_, err := reader.Read(context.Background(), exec)
	if err == nil {
		t.Errorf("expected error, have: %v", err)
	}
}

func TestYAMLConfig(t *testing.T) {
	contents := []byte(`# .gopherci.yml config
apt_packages:
    - package1
`)
	exec := &mockAnalyser{
		ExecuteOut: [][]byte{contents},
		ExecuteErr: []error{nil},
	}

	reader := &YAMLConfig{
		Tools: []db.Tool{{Name: "tool1"}},
	}
	have, err := reader.Read(context.Background(), exec)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	want := RepoConfig{
		APTPackages: []string{"package1"},
		Tools:       reader.Tools,
	}

	if !reflect.DeepEqual(have, want) {
		t.Errorf("\nhave: %v\nwant: %v", have, want)
	}
}

package analyser

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestPullRequestCloner(t *testing.T) {
	cloner := &PullRequestCloner{
		HeadRef: "head-ref",
		HeadURL: "head-url",
		BaseRef: "base-ref",
		BaseURL: "base-url",
	}

	passExec := &mockAnalyser{
		ExecuteOut: [][]byte{{}, {}},
		ExecuteErr: []error{nil, nil},
	}
	passArgs := [][]string{
		{"git", "clone", "--depth", "1", "--branch", cloner.HeadRef, "--single-branch", cloner.HeadURL, "."},
		{"git", "fetch", "--depth", "1", cloner.BaseURL, cloner.BaseRef},
	}

	// clone failed
	cloneFailExec := &mockAnalyser{
		ExecuteOut: [][]byte{{}},
		ExecuteErr: []error{errors.New("clone fail")},
	}
	cloneFailErr := errors.New(`could not execute [git clone --depth 1 --branch head-ref --single-branch head-url .]: "": clone fail`)

	// fetch failed
	fetchFailExec := &mockAnalyser{
		ExecuteOut: [][]byte{{}, {}},
		ExecuteErr: []error{nil, errors.New("fetch fail")},
	}
	fetchFailErr := errors.New(`could not execute [git fetch --depth 1 base-url base-ref]: "": fetch fail`)

	tests := []struct {
		executer *mockAnalyser
		wantArgs [][]string // nil to not check for args
		wantErr  error
	}{
		{passExec, passArgs, nil},
		{cloneFailExec, nil, cloneFailErr},
		{fetchFailExec, nil, fetchFailErr},
	}

	for _, test := range tests {
		err := cloner.Clone(context.Background(), test.executer)
		if err != test.wantErr && err.Error() != test.wantErr.Error() {
			t.Errorf("\nhave: %v\nwant: %v", err, test.wantErr)
		}

		if test.wantArgs != nil && !reflect.DeepEqual(test.executer.Executed, test.wantArgs) {
			t.Errorf("\nhave: %v\nwant: %v", test.executer.Executed, test.wantArgs)
		}
	}
}

func TestPushCloner(t *testing.T) {
	cloner := &PushCloner{
		HeadRef: "head-ref",
		HeadURL: "head-url",
	}

	passExec := &mockAnalyser{
		ExecuteOut: [][]byte{{}, {}},
		ExecuteErr: []error{nil, nil},
	}
	passArgs := [][]string{
		{"git", "clone", cloner.HeadURL, "."},
		{"git", "checkout", cloner.HeadRef},
	}

	// clone failed
	cloneFailExec := &mockAnalyser{
		ExecuteOut: [][]byte{{}},
		ExecuteErr: []error{errors.New("clone fail")},
	}
	cloneFailErr := errors.New(`could not execute [git clone head-url .]: "": clone fail`)

	// checkout failed
	coFailExec := &mockAnalyser{
		ExecuteOut: [][]byte{{}, {}},
		ExecuteErr: []error{nil, errors.New("checkout fail")},
	}
	coFailErr := errors.New(`could not execute [git checkout head-ref]: "": checkout fail`)

	tests := []struct {
		executer *mockAnalyser
		wantArgs [][]string // nil to not check for args
		wantErr  error
	}{
		{passExec, passArgs, nil},
		{cloneFailExec, nil, cloneFailErr},
		{coFailExec, nil, coFailErr},
	}

	for _, test := range tests {
		err := cloner.Clone(context.Background(), test.executer)
		if err != test.wantErr && err.Error() != test.wantErr.Error() {
			t.Errorf("\nhave: %v\nwant: %v", err, test.wantErr)
		}

		if test.wantArgs != nil && !reflect.DeepEqual(test.executer.Executed, test.wantArgs) {
			t.Errorf("\nhave: %v\nwant: %v", test.executer.Executed, test.wantArgs)
		}
	}
}

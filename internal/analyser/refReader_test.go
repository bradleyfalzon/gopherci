package analyser

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestMergeBase(t *testing.T) {

	refReader := &MergeBase{}

	passExec := &mockExecuter{
		ExecuteOut: [][]byte{[]byte("abcdef\n")},
		ExecuteErr: []error{nil},
	}
	passArgs := [][]string{
		{"git", "merge-base", "FETCH_HEAD", "HEAD"},
	}

	readerFailExec := &mockExecuter{
		ExecuteOut: [][]byte{{}},
		ExecuteErr: []error{errors.New("git merge-base fail")},
	}
	readerFailErr := errors.New(`could not execute [git merge-base FETCH_HEAD HEAD]: "": git merge-base fail`)

	tests := []struct {
		executer *mockExecuter
		wantArgs [][]string // nil to not check for args
		wantErr  error
	}{
		{passExec, passArgs, nil},
		{readerFailExec, nil, readerFailErr},
	}

	for _, test := range tests {
		have, err := refReader.Base(context.Background(), test.executer)
		if err != test.wantErr && err.Error() != test.wantErr.Error() {
			t.Errorf("\nhave: %v\nwant: %v", err, test.wantErr)
		}

		if test.wantArgs != nil && !reflect.DeepEqual(test.executer.Executed, test.wantArgs) {
			t.Errorf("\nhave: %v\nwant: %v", test.executer.Executed, test.wantArgs)
		}

		if want := "abcdef"; err == nil && want != have {
			t.Errorf("have: %v, want: %v", have, want)
		}
	}
}

func TestFixedRef(t *testing.T) {
	refReader := &FixedRef{BaseRef: "abcdef"}

	have, err := refReader.Base(context.Background(), nil)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if want := "abcdef"; have != want {
		t.Errorf("have: %v, want: %v", have, want)
	}
}

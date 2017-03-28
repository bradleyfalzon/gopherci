package db

import (
	"testing"
	"time"
)

func TestAnalysisStatus_scan(t *testing.T) {
	tests := []struct {
		input interface{}
		want  AnalysisStatus
		err   error
	}{
		{nil, AnalysisStatusPending, nil},
		{[]uint8("Pending"), AnalysisStatusPending, nil},
		{[]uint8("Failure"), AnalysisStatusFailure, nil},
		{[]uint8("Success"), AnalysisStatusSuccess, nil},
		{[]uint8("Error"), AnalysisStatusError, nil},
		{[]uint8("NA"), "", errUnknownAnalysis},
	}

	for _, test := range tests {
		var status AnalysisStatus
		err := status.Scan(test.input)
		if err != test.err {
			t.Errorf("unexpected error: have: %v want: %v", err, test.err)
		}
		if status != test.want {
			t.Errorf("input: %#v have: %#v want %#v", test.input, status, test.want)
		}
	}
}

func TestDuration_scan(t *testing.T) {
	tests := []struct {
		input   interface{}
		want    Duration
		isError bool
	}{
		{nil, 0, false},
		{[]uint8("01:02:03"), Duration(1*time.Hour + 2*time.Minute + 3*time.Second), false},
		{[]uint8("00:00:03.100"), Duration(3*time.Second + 100*time.Millisecond), false},
		{[]uint8("unknown format"), 0, true},
	}

	for _, test := range tests {
		var d Duration
		err := d.Scan(test.input)
		if err != nil && !test.isError || err == nil && test.isError {
			t.Errorf("unexpected error: %v", err)
		}
		if d != test.want {
			t.Errorf("input: %s have: %#v want %#v", test.input, d, test.want)
		}
	}
}

func TestDuration_value(t *testing.T) {
	want := 1.100
	have, err := Duration(time.Second + 100*time.Millisecond).Value()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if have != want {
		t.Errorf("have: %v, want: %v", have, want)
	}
}

func TestDuration_string(t *testing.T) {
	want := time.Duration(100).String()
	have := Duration(100).String()
	if have != want {
		t.Errorf("have: %v, want: %v", have, want)
	}
}

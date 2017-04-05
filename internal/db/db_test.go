package db

import (
	"fmt"
	"reflect"
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

func TestAnalysis_issues(t *testing.T) {
	analysis := NewAnalysis()
	analysis.Tools[1] = AnalysisTool{
		Issues: []Issue{{Issue: "issue"}},
	}
	analysis.Tools[2] = AnalysisTool{
		Issues: []Issue{{Issue: "issue"}},
	}

	want := []Issue{{Issue: "issue"}, {Issue: "issue"}}
	if have := analysis.Issues(); !reflect.DeepEqual(have, want) {
		t.Errorf("\nhave: %#v\nwant: %#v", have, want)
	}
}

func TestAnalysis_htmlurl(t *testing.T) {
	analysis := NewAnalysis()
	analysis.ID = 10
	want := fmt.Sprintf("https://example.com/analysis/%d", analysis.ID)
	if have := analysis.HTMLURL("https://example.com"); have != want {
		t.Errorf("have: %q, want: %q", have, want)
	}
}

func TestAnalysis_isPush(t *testing.T) {
	tests := []struct {
		RequestNumber int
		CommitFrom    string
		CommitTo      string
		IsPush        bool
	}{
		{10, "", "", false},
		{0, "aaa", "bbb", true},
	}

	for _, test := range tests {
		analysis := NewAnalysis()
		analysis.RequestNumber = test.RequestNumber
		analysis.CommitFrom = test.CommitFrom
		analysis.CommitTo = test.CommitTo

		if have := analysis.IsPush(); have != test.IsPush {
			t.Errorf("have: %v, want: %v test: %#v", have, test.IsPush, test)
		}
	}
}

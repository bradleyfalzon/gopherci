package github

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"

	"github.com/google/go-github/github"
)

func TestInstallation_isEnabled(t *testing.T) {
	var i *Installation
	if i.IsEnabled() {
		t.Errorf("want: %v, have: %v", true, i.IsEnabled())
	}
	i = &Installation{}
	if !i.IsEnabled() {
		t.Errorf("want: %v, have: %v", false, i.IsEnabled())
	}
}

func TestInstallation_diff(t *testing.T) {
	var (
		wantDiff = []byte("diff")
		api      []byte
	)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.RequestURI {
		case "/repositories/11/pulls/10":
			// API response for pull requests
			w.Write(api)
		case "/repositories/11/compare/zzzct":
			// API response for first pushes
			w.Write(api)
		case "/repositories/11/compare/zzzct~3...zzzct":
			// API response for pushes
			w.Write(api)
		case "/diff.diff":
			w.Write(wantDiff)
		default:
			t.Logf(r.RequestURI)
		}
	}))
	defer ts.Close()

	api = []byte(fmt.Sprintf(`{"diff_url": "%v/diff.diff"}`, ts.URL))
	i := Installation{client: github.NewClient(nil)}
	i.client.BaseURL, _ = url.Parse(ts.URL)

	tests := []struct {
		commitFrom string
		commitTo   string
		requestNum int
	}{
		{"zzzct~3", "zzzct", 0},
		{"", "", 10},
	}

	for _, test := range tests {
		body, err := i.Diff(context.Background(), 11, test.commitFrom, test.commitTo, test.requestNum)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		haveDiff, err := ioutil.ReadAll(body)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !reflect.DeepEqual(haveDiff, wantDiff) {
			t.Errorf("diff have: %s, want: %s", haveDiff, wantDiff)
		}
	}
}

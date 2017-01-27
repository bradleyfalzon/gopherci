package analyser

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/bradleyfalzon/gopherci/internal/db"
)

type mockAnalyser struct {
	Executed   [][]string
	ExecuteOut [][]byte
	Stopped    bool
}

var _ Analyser = &mockAnalyser{}
var _ Executer = &mockAnalyser{}

func (a *mockAnalyser) NewExecuter() (Executer, error) {
	// Return itself
	return a, nil
}

func (a *mockAnalyser) Execute(args []string) (out []byte, err error) {
	a.Executed = append(a.Executed, args)
	out, a.ExecuteOut = a.ExecuteOut[0], a.ExecuteOut[1:]
	return out, err
}

func (a *mockAnalyser) Stop() error {
	a.Stopped = true
	return nil
}

func TestAnalyse(t *testing.T) {
	var diffFetched bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		diffFetched = true
		fmt.Fprintln(w, `diff --git a/subdir/main.go b/subdir/main.go
new file mode 100644
index 0000000..6362395
--- /dev/null
+++ b/main.go
@@ -0,0 +1,1 @@
+var _ = fmt.Sprintln()`)
	}))
	defer ts.Close()

	cfg := Config{
		BaseURL:    "base-url",
		BaseBranch: "base-branch",
		HeadURL:    "head-url",
		HeadBranch: "head-branch",
		DiffURL:    ts.URL,
	}

	tools := []db.Tool{
		{Name: "Name1", Path: "tool1", Args: "-flag %BASE_BRANCH% ./..."},
		{Name: "Name2", Path: "tool2"},
	}

	analyser := &mockAnalyser{
		ExecuteOut: [][]byte{
			{}, // git clone
			{}, // git fetch
			{}, // install-deps.sh
			[]byte(`/go/src/gopherci`),                   // pwd
			[]byte("main.go:1: error1"),                  // tool 1
			[]byte("/go/src/gopherci/main.go:1: error2"), // tool 2 output abs paths
		},
	}

	issues, err := Analyse(analyser, tools, cfg)
	if err != nil {
		t.Fatal("unexpected error:", err)
	}

	expected := []Issue{
		{File: "main.go", HunkPos: 1, Issue: "Name1: error1"},
		{File: "main.go", HunkPos: 1, Issue: "Name2: error2"},
	}
	if !reflect.DeepEqual(expected, issues) {
		t.Errorf("expected issues:\n%+v\ngot:\n%+v", expected, issues)
	}

	if !diffFetched {
		t.Errorf("expected diff to be fetched")
	}

	if !analyser.Stopped {
		t.Errorf("expected analyser to be stopped")
	}

	expectedArgs := [][]string{
		{"git", "clone", "--branch", "head-branch", "--depth", "1", "--single-branch", "head-url", "."},
		{"git", "fetch", cfg.BaseURL, cfg.BaseBranch},
		{"install-deps.sh"},
		{"pwd"},
		{"tool1", "-flag", "FETCH_HEAD", "./..."},
		{"tool2"},
	}

	if !reflect.DeepEqual(analyser.Executed, expectedArgs) {
		t.Errorf("\nhave %v\nwant %v", analyser.Executed, expectedArgs)
	}
}

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
		{Name: "Name", Path: "tool", Args: "-flag %BASE_BRANCH% ./..."},
	}

	analyser := &mockAnalyser{
		ExecuteOut: [][]byte{{}, {}, []byte(`main.go:1: error`)},
	}

	issues, err := Analyse(analyser, tools, cfg)
	if err != nil {
		t.Fatal("unexpected error:", err)
	}

	expected := []Issue{{File: "main.go", HunkPos: 1, Issue: "Name: error"}}
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
		{"git", "clone", "--branch", "head-branch", "--depth", "0", "--single-branch", "head-url", "."},
		{"git", "fetch", cfg.BaseURL, cfg.BaseBranch},
		{"tool", "-flag", "FETCH_HEAD", "./..."},
	}

	if !reflect.DeepEqual(analyser.Executed, expectedArgs) {
		t.Errorf("have %v\nwant %v", analyser.Executed, expectedArgs)
	}
}

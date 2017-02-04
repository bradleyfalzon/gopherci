package analyser

import (
	"fmt"
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

func (a *mockAnalyser) NewExecuter(_ string) (Executer, error) {
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

func TestAnalyse_pr(t *testing.T) {
	cfg := Config{
		EventType: EventTypePullRequest,
		BaseURL:   "base-url",
		BaseRef:   "base-branch",
		HeadURL:   "head-url",
		HeadRef:   "head-branch",
	}

	tools := []db.Tool{
		{Name: "Name1", Path: "tool1", Args: "-flag %BASE_BRANCH% ./..."},
		{Name: "Name2", Path: "tool2"},
	}

	diff := []byte(`diff --git a/subdir/main.go b/subdir/main.go
new file mode 100644
index 0000000..6362395
--- /dev/null
+++ b/main.go
@@ -0,0 +1,1 @@
+var _ = fmt.Sprintln()`)

	analyser := &mockAnalyser{
		ExecuteOut: [][]byte{
			{},   // git clone
			{},   // git fetch
			diff, // git diff
			{},   // install-deps.sh
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

	if !analyser.Stopped {
		t.Errorf("expected analyser to be stopped")
	}

	expectedArgs := [][]string{
		{"git", "clone", "--depth", "1", "--branch", cfg.HeadRef, "--single-branch", cfg.HeadURL, "."},
		{"git", "fetch", "--depth", "1", cfg.BaseURL, cfg.BaseRef},
		{"git", "diff", fmt.Sprintf("FETCH_HEAD...%v", cfg.HeadRef)},
		{"install-deps.sh"},
		{"pwd"},
		{"tool1", "-flag", "FETCH_HEAD", "./..."},
		{"tool2"},
	}

	if !reflect.DeepEqual(analyser.Executed, expectedArgs) {
		t.Errorf("\nhave %v\nwant %v", analyser.Executed, expectedArgs)
	}
}

func TestAnalyse_push(t *testing.T) {
	cfg := Config{
		EventType: EventTypePush,
		BaseURL:   "base-url",
		BaseRef:   "abcde~1",
		HeadURL:   "head-url",
		HeadRef:   "abcde",
	}

	tools := []db.Tool{
		{Name: "Name1", Path: "tool1", Args: "-flag %BASE_BRANCH% ./..."},
		{Name: "Name2", Path: "tool2"},
	}

	diff := []byte(`diff --git a/subdir/main.go b/subdir/main.go
new file mode 100644
index 0000000..6362395
--- /dev/null
+++ b/main.go
@@ -0,0 +1,1 @@
+var _ = fmt.Sprintln()`)

	analyser := &mockAnalyser{
		ExecuteOut: [][]byte{
			{},   // git clone
			{},   // git checkout
			diff, // git diff
			{},   // install-deps.sh
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

	if !analyser.Stopped {
		t.Errorf("expected analyser to be stopped")
	}

	expectedArgs := [][]string{
		{"git", "clone", cfg.HeadURL, "."},
		{"git", "checkout", cfg.HeadRef},
		{"git", "diff", fmt.Sprintf("%v...%v", cfg.BaseRef, cfg.HeadRef)},
		{"install-deps.sh"},
		{"pwd"},
		{"tool1", "-flag", "abcde~1", "./..."},
		{"tool2"},
	}

	if !reflect.DeepEqual(analyser.Executed, expectedArgs) {
		t.Errorf("\nhave %v\nwant %v", analyser.Executed, expectedArgs)
	}
}

func TestAnalyse_unknown(t *testing.T) {
	cfg := Config{}
	analyser := &mockAnalyser{}
	_, err := Analyse(analyser, nil, cfg)
	if err == nil {
		t.Fatal("expected error got nil")
	}
}

package analyser

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

// mockExecuter is a poor implementation of mock object and really should
// be replaced with an external library
type mockExecuter struct {
	t *testing.T
	// coArgsIn holds a slice of arguments for each invocation to require
	coArgsIn [][]string
	// coOut holds an array of each invocations output to return
	coOut [][]byte
}

var _ executer = (*mockExecuter)(nil)

func (e *mockExecuter) CombinedOutput(cmd *exec.Cmd) ([]byte, error) {
	var (
		output       []byte
		expectedArgs []string
	)
	expectedArgs, e.coArgsIn = e.coArgsIn[0], e.coArgsIn[1:]
	if !reflect.DeepEqual(expectedArgs, cmd.Args) {
		e.t.Fatalf("expected args\n%+v\ngot\n%+v", expectedArgs, cmd.Args)
	}

	output, e.coOut = e.coOut[0], e.coOut[1:]
	return output, nil
}

func (mockExecuter) Mktemp(base string) (string, error) {
	return filepath.Join(base, "rand"), nil
}

func TestAnalyse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `diff --git a/subdir/main.go b/subdir/main.go
new file mode 100644
index 0000000..6362395
--- /dev/null
+++ b/main.go
@@ -0,0 +1,1 @@
+var _ = fmt.Sprintln()`)
	}))
	defer ts.Close()

	executer := &mockExecuter{
		t: t,
		coArgsIn: [][]string{
			{"git", "clone", "--branch", "some-branch", "--depth", "0", "--single-branch", "repo-url", "/tmp/src/gopherci/rand"},
			{"go", "vet", "./..."},
		},
		coOut: [][]byte{{}, []byte(`main.go:1: error`)},
	}

	fs, err := NewFileSystem("/tmp")
	if err != nil {
		t.Fatal("unexpected error:", err)
	}
	fs.executer = executer

	issues, err := fs.Analyse("repo-url", "some-branch", ts.URL)

	if err != nil {
		t.Fatal("unexpected error:", err)
	}

	expected := []Issue{{File: "main.go", HunkPos: 1, Issue: "main.go:1: error"}}
	if !reflect.DeepEqual(expected, issues) {
		t.Errorf("expected issues:\n%+v\ngot:\n%+v", expected, issues)
	}
}

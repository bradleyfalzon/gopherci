package analyser

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/bradleyfalzon/gopherci/internal/db"
	"github.com/pkg/errors"
)

// Analyser analyses a repository and branch, returns issues found in patch
// or an error.
type Analyser interface {
	Analyse(tools []db.Tool, repoURL, branch, diffURL string) ([]Issue, error)
}

// Issue contains file, position and string describing a single issue.
type Issue struct {
	// File is the relative path name of the file.
	File string
	// HunkPos is the position relative to the files first hunk.
	HunkPos int
	// Issue is the issue.
	Issue string
}

// executer is an interface used for mocking real file system calls
type executer interface {
	// CombinedOutput executes CombinedOutput on provided cmd.
	CombinedOutput(*exec.Cmd) ([]byte, error)
	// Run executes Run on provded cmd.
	Run(*exec.Cmd) error
	// Mktemp makes and returns full path to a random directory inside absolute path base.
	Mktemp(string) (string, error)
}

type fsExecuter struct{}

// Ensure fsExecuter implements executer.
var _ executer = (*fsExecuter)(nil)

// CombinedOutput implements executer interface
func (fsExecuter) CombinedOutput(cmd *exec.Cmd) ([]byte, error) {
	return cmd.CombinedOutput()
}

// Run implements executer interface
func (fsExecuter) Run(cmd *exec.Cmd) error {
	return cmd.Run()
}

// Mktemp implements executer interface
func (fsExecuter) Mktemp(base string) (string, error) {
	rand := strconv.Itoa(int(time.Now().UnixNano()))
	dir := filepath.Join(base, rand)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", errors.Wrap(err, "fsExecuter.Mktemp: cannot mkdir")
	}
	return dir, nil
}

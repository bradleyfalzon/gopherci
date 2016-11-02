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

const (
	// ArgBaseBranch replaces tool arg with the name of the base branch
	ArgBaseBranch = "%BASE_BRANCH%"
)

// Analyser analyses a repository and branch, returns issues found in patch
// or an error.
type Analyser interface {
	Analyse(tools []db.Tool, config Config) ([]Issue, error)
}

// Config hold configuration options for use in analyser. All options
// are required.
type Config struct {
	// BaseURL is the VCS fetchable base repo URL.
	BaseURL string
	// BaseBranch is the branch we want to merge into.
	BaseBranch string
	// HeadURL is the VCS fetchable repo URL containing the changes to be merged.
	HeadURL string
	// HeadBranch is the name of the branch containing changes.
	HeadBranch string
	// DiffURL is the URL containing the unified diff of the changes.
	DiffURL string
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

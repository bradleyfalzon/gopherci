package analyser

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/bradleyfalzon/gopherci/internal/db"
	"github.com/bradleyfalzon/revgrep"
	"github.com/pkg/errors"
)

const (
	// ArgBaseBranch replaces tool arg with the name of the base branch
	ArgBaseBranch = "%BASE_BRANCH%"
)

// An Analyser is builds an isolated execution environment to run checks in.
// It should provide isolation from other environments and support being
// called concurrently.
type Analyser interface {
	// NewExecuter returns an Executer with the working directory set to
	// $GOPATH/src/<goSrcPath>.
	NewExecuter(goSrcPath string) (Executer, error)
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
	// GoSrcPath is the repository's path when placed in $GOPATH/src.
	GoSrcPath string
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

// Executer executes a single command in a contained environment.
type Executer interface {
	// Execute executes a command and returns the combined stdout and stderr,
	// along with an error if any. Must not be called after Stop().
	Execute([]string) ([]byte, error)
	// Stop stops the executer and allows it to cleanup, if applicable.
	Stop() error
}

// Analyse downloads a repository set in config in an environment provided by
// analyser, running the series of tools. Returns issues from tools that have
// are likely to have been caused by a change.
func Analyse(analyser Analyser, tools []db.Tool, config Config) ([]Issue, error) {

	// download patch
	resp, err := http.Get(config.DiffURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	patch, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "could not ioutil.ReadAll response from"+config.DiffURL)
	}

	// Get a new executer/environment to execute in
	exec, err := analyser.NewExecuter(config.GoSrcPath)
	if err != nil {
		return nil, errors.Wrap(err, "analyser could create new executer")
	}

	// clone repo
	// TODO check out https://godoc.org/golang.org/x/tools/go/vcs to be agnostic
	args := []string{"git", "clone", "--branch", config.HeadBranch, "--depth", "1", "--single-branch", config.HeadURL, "."}
	out, err := exec.Execute(args)
	if err != nil {
		return nil, fmt.Errorf("could not execute %v: %s\n%s", args, err, out)
	}

	// fetch base/upstream as some tools (apicompat) needs it
	args = []string{"git", "fetch", config.BaseURL, config.BaseBranch}
	out, err = exec.Execute(args)
	if err != nil {
		return nil, fmt.Errorf("could not execute %v: %s\n%s", args, err, out)
	}

	// install dependencies, some static analysis tools require building a project
	args = []string{"install-deps.sh"}
	out, err = exec.Execute(args)
	if err != nil {
		return nil, fmt.Errorf("could not execute %v: %s\n%s", args, err, out)
	}
	log.Printf("install-deps.sh output: %s", bytes.TrimSpace(out))

	// get the base package working directory, used by revgrep to change absolute
	// path for the filename in an issue (used by some tools) to relative (used by
	// patch).
	args = []string{"pwd"}
	out, err = exec.Execute(args)
	if err != nil {
		return nil, fmt.Errorf("could not execute %v: %s\n%s", args, err, out)
	}
	pwd := string(bytes.TrimSpace(out))

	var issues []Issue
	for _, tool := range tools {
		args := []string{tool.Path}
		for _, arg := range strings.Fields(tool.Args) {
			switch arg {
			case ArgBaseBranch:
				// Tool wants the base branch name as a flag
				arg = "FETCH_HEAD"
			}
			args = append(args, arg)
		}
		// ignore errors, often it's about the exit status
		// TODO check these errors better, other static analysis tools check the code
		// explicitly or at least don't ignore it
		out, _ := exec.Execute(args)
		log.Printf("%v output:\n%s", tool.Name, out)

		checker := revgrep.Checker{
			Patch:   bytes.NewReader(patch),
			Regexp:  tool.Regexp,
			Debug:   os.Stdout,
			AbsPath: pwd,
		}

		revIssues, err := checker.Check(bytes.NewReader(out), ioutil.Discard)
		if err != nil {
			return nil, err
		}
		log.Printf("revgrep found %v issues", len(revIssues))

		for _, issue := range revIssues {
			issues = append(issues, Issue{
				File:    issue.File,
				HunkPos: issue.HunkPos,
				Issue:   fmt.Sprintf("%s: %s", tool.Name, issue.Message),
			})
		}
	}

	if err := exec.Stop(); err != nil {
		log.Printf("warning: could not stop executer: %v", err)
	}

	return issues, nil
}

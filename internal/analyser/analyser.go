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

// Analyser analyses a repository and branch, returns issues found in patch
// or an error.
type Analyser interface {
	NewExecuter() (Executer, error)
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

// Executer executes a single command in a contained environment.
type Executer interface {
	// Execute executes a command and returns the combined stdout and stderr,
	// along with an error if any. Must not be called after Stop().
	Execute([]string) ([]byte, error)
	// Stop stops the executer and allows it to cleanup, if applicable.
	Stop() error
}

// Analyse downloads a repository set in config in an envrionment provided by
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
	exec, err := analyser.NewExecuter()

	// clone repo
	// TODO check out https://godoc.org/golang.org/x/tools/go/vcs to be agnostic
	args := []string{"git", "clone", "--branch", config.HeadBranch, "--depth", "0", "--single-branch", config.HeadURL, "."}
	out, err := exec.Execute(args)
	if err != nil {
		return nil, fmt.Errorf("could not execute %v: %s\n%s", args, err, out)
	}
	log.Println("git clone success")

	// fetch base/upstream as some tools (apicompat) needs it
	args = []string{"git", "fetch", config.BaseURL, config.BaseBranch}
	out, err = exec.Execute(args)
	if err != nil {
		return nil, fmt.Errorf("could not execute %v: %s\n%s", args, err, out)
	}
	log.Println("fetch base success")

	// fetch dependencies, some static analysis tools require building a project

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
		log.Printf("tool: %v, args: %v", tool.Name, args)
		// ignore errors, often it's about the exit status
		// TODO check these errors better, other static analysis tools check the code
		// explicitly or at least don't ignore it
		out, _ := exec.Execute(args)
		log.Printf("%v output:\n%s", tool.Name, out)

		checker := revgrep.Checker{
			Patch:  bytes.NewReader(patch),
			Regexp: tool.Regexp,
			Debug:  os.Stdout,
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

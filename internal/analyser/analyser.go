package analyser

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
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
	// EventType defines the type of event being processed.
	EventType EventType
	// BaseURL is the VCS fetchable base repo URL.
	BaseURL string
	// BaseRef is the reference we want to merge into, for EventTypePullRequest
	// it's likely the branch, for EventTypePush it's a sha~number.
	BaseRef string
	// HeadURL is the VCS fetchable repo URL containing the changes to be merged.
	HeadURL string
	// HeadRef is the name of the reference containing changes.
	HeadRef string
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

// EventType defines the type of even which needs to be analysed, as there
// maybe different or optimal methods based on the type.
type EventType int

const (
	// EventTypeUnknown cannot be handled and is the zero value for an EventType.
	EventTypeUnknown EventType = iota
	// EventTypePullRequest is a Pull Request.
	EventTypePullRequest
	// EventTypePush is a push.
	EventTypePush
)

// Analyse downloads a repository set in config in an environment provided by
// analyser, running the series of tools. Returns issues from tools that have
// are likely to have been caused by a change.
func Analyse(analyser Analyser, tools []db.Tool, config Config) ([]Issue, error) {
	// Get a new executer/environment to execute in
	exec, err := analyser.NewExecuter(config.GoSrcPath)
	if err != nil {
		return nil, errors.Wrap(err, "analyser could create new executer")
	}

	// baseRef is the reference to the base branch or before commit, the ref
	// of the state before this PR/Push.
	var baseRef string
	switch config.EventType {
	case EventTypePullRequest:
		// clone repo
		args := []string{"git", "clone", "--depth", "1", "--branch", config.HeadRef, "--single-branch", config.HeadURL, "."}
		out, err := exec.Execute(args)
		if err != nil {
			return nil, fmt.Errorf("could not execute %v: %s\n%s", args, err, out)
		}

		// This is a PR, fetch base as some tools (apicompat) needs to
		// reference it.
		args = []string{"git", "fetch", "--depth", "1", config.BaseURL, config.BaseRef}
		out, err = exec.Execute(args)
		if err != nil {
			return nil, fmt.Errorf("could not execute %v: %s\n%s", args, err, out)
		}
		baseRef = "FETCH_HEAD"
	case EventTypePush:
		// clone repo, this cannot be shallow and needs access to all commits
		// therefore cannot be shallow (or if it is, would required a very
		// large depth and --no-single-branch).
		args := []string{"git", "clone", config.HeadURL, "."}
		out, err := exec.Execute(args)
		if err != nil {
			return nil, fmt.Errorf("could not execute %v: %s\n%s", args, err, out)
		}

		// Checkout sha
		args = []string{"git", "checkout", config.HeadRef}
		out, err = exec.Execute(args)
		if err != nil {
			return nil, fmt.Errorf("could not execute %v: %s\n%s", args, err, out)
		}
		baseRef = config.BaseRef
	default:
		return nil, errors.Errorf("unknown event type %T", config.EventType)
	}

	// create a unified diff for use by revgrep
	args := []string{"git", "diff", fmt.Sprintf("%v...%v", baseRef, config.HeadRef)}
	patch, err := exec.Execute(args)
	if err != nil {
		return nil, fmt.Errorf("could not execute %v: %s\n%s", args, err, patch)
	}
	log.Printf("%v output: %s", args, bytes.TrimSpace(patch))

	// install dependencies, some static analysis tools require building a project
	args = []string{"install-deps.sh"}
	out, err := exec.Execute(args)
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
			case ArgBaseBranch: // TODO change to ArgBaseRef
				// Tool wants the base ref name as a flag
				arg = baseRef
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

	log.Printf("stopping executer")
	if err := exec.Stop(); err != nil {
		log.Printf("warning: could not stop executer: %v", err)
	}
	log.Printf("finished stopping executer")

	return issues, nil
}

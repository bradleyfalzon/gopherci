package analyser

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	"github.com/bradleyfalzon/gopherci/internal/db"
	"github.com/bradleyfalzon/gopherci/internal/logger"
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
	NewExecuter(ctx context.Context, goSrcPath string) (Executer, error)
}

// Config hold configuration options for use in analyser. All options
// are required.
type Config struct {
	// HeadRef is the name of the reference containing changes.
	HeadRef string
}

// Executer executes a single command in a contained environment.
type Executer interface {
	// Execute executes a command and returns the combined stdout and stderr,
	// along with an error if any. Must not be called after Stop(). If the
	// command returns a non-zero exit code, an error of type NonZeroError
	// is returned.
	Execute(context.Context, []string) ([]byte, error)
	// Stop stops the executer and allows it to cleanup, if applicable.
	Stop(context.Context) error
}

// NonZeroError maybe returned by an Executer when the command executed returns
// with a non-zero exit status.
type NonZeroError struct {
	args     []string
	ExitCode int // ExitCode is the non zero exit code
}

// Error implements the error interface.
func (e *NonZeroError) Error() string {
	return fmt.Sprintf("%v returned exit code %v", e.args, e.ExitCode)
}

// Analyse downloads a repository set in config in an environment provided by
// exec, running the series of tools. Writes results to provided analysis,
// or an error. The repository is expected to contain at least one Go package.
func Analyse(ctx context.Context, logger logger.Logger, exec Executer, cloner Cloner, configReader ConfigReader, refReader RefReader, config Config, analysis *db.Analysis) error {
	start := time.Now()
	defer func() {
		analysis.TotalDuration = db.Duration(time.Since(start))
	}()
	logger = logger.With("area", "analyser")

	deltaStart := time.Now() // start of specific analysis
	if err := cloner.Clone(ctx, exec); err != nil {
		return errors.WithMessage(err, "could not clone")
	}
	analysis.CloneDuration = db.Duration(time.Since(deltaStart))

	// read repository's configuration
	repoConfig, err := configReader.Read(ctx, exec)
	if err != nil {
		return errors.WithMessage(err, "could not configure repository")
	}

	// Show environment
	envArgs := [][]string{
		{"go", "env"},
		{"go", "version"},
		{"cat", "/proc/self/limits"},
		{"lsb_release", "--description"},
	}
	for _, arg := range envArgs {
		out, err := exec.Execute(ctx, arg)
		if err != nil {
			return fmt.Errorf("could not execute %v: %s\n%s", arg, err, out)
		}
	}

	// install packages
	if err := installAPTPackages(ctx, exec, repoConfig.APTPackages); err != nil {
		return errors.WithMessage(err, "could not install packages")
	}

	// get the base ref
	baseRef, err := refReader.Base(ctx, exec)
	if err != nil {
		return errors.Wrap(err, "could not get base ref")
	}

	// create a unified diff for use by revgrep
	patch, err := getPatch(ctx, exec, baseRef, config.HeadRef)
	if err != nil {
		return errors.Wrap(err, "could not get patch")
	}

	// install dependencies, some static analysis tools require building a project
	deltaStart = time.Now()
	args := []string{"install-deps.sh"}
	out, err := exec.Execute(ctx, args)
	if err != nil {
		return fmt.Errorf("could not execute %v: %s\n%s", args, err, out)
	}
	analysis.DepsDuration = db.Duration(time.Since(deltaStart))
	logger.With("step", "install-deps.sh").Info(string(bytes.TrimSpace(out)))

	// get the base package working directory, used by revgrep to change absolute
	// path for the filename in an issue (used by some tools) to relative (used by
	// patch).
	args = []string{"pwd"}
	out, err = exec.Execute(ctx, args)
	if err != nil {
		return fmt.Errorf("could not execute %v: %s\n%s", args, err, out)
	}
	pwd := string(bytes.TrimSpace(out))

	for _, tool := range repoConfig.Tools {
		deltaStart = time.Now()
		args := []string{tool.Path}
		for _, arg := range strings.Fields(tool.Args) {
			switch arg {
			case ArgBaseBranch: // TODO change to ArgBaseRef
				// Tool wants the base ref name as a flag
				arg = baseRef
			}
			args = append(args, arg)
		}
		out, err := exec.Execute(ctx, args)
		switch err.(type) {
		case nil, *NonZeroError:
			// Ignore non-zero exit codes from tools, these are often normal.
		default:
			return fmt.Errorf("could not execute %v: %s\n%s", args, err, out)
		}
		logger.With("step", tool.Name).Info("ran tool")

		checker := revgrep.Checker{
			Patch:   bytes.NewReader(patch),
			Regexp:  tool.Regexp,
			AbsPath: pwd,
		}

		revIssues, err := checker.Check(bytes.NewReader(out), ioutil.Discard)
		if err != nil {
			return err
		}
		logger.Infof("revgrep found %v issues", len(revIssues))

		var issues []db.Issue
		for _, issue := range revIssues {
			// Remove issues in generated files, isFileGenereated will return
			// 0 for file is generated or 1 for file is not generated.
			args = []string{"isFileGenerated", pwd, issue.File}
			out, err := exec.Execute(ctx, args)
			logger.With("step", "isFileGenerated").Info(string(bytes.TrimSpace(out)))
			switch err {
			case nil:
				continue // file is generated, ignore the issue
			default:
				if etype, ok := err.(*NonZeroError); ok && etype.ExitCode == 1 {
					break // file is not generated, record the issue
				}
				return fmt.Errorf("could not execute %v: %s\n%s", args, err, out)
			}

			issues = append(issues, db.Issue{
				Path:    issue.File,
				Line:    issue.LineNo,
				HunkPos: issue.HunkPos,
				Issue:   fmt.Sprintf("%s: %s", tool.Name, issue.Message),
			})
		}

		analysis.Tools[tool.ID] = db.AnalysisTool{
			Duration: db.Duration(time.Since(deltaStart)),
			Issues:   issues,
		}
	}

	return nil
}

func getPatch(ctx context.Context, exec Executer, baseRef, headRef string) ([]byte, error) {
	args := []string{"git", "diff", fmt.Sprintf("%v...%v", baseRef, headRef)}
	patch, err := exec.Execute(ctx, args)
	if err != nil {
		// The error may be because baseRef does not exist
		// - remote ref has been removed (but then the clone wouldn't have worked)
		// - new repository with zero history
		// - a new branch with no shared history
		// So use git show to generate a unified diff of just the latest ref.
		var (
			showArgs = []string{"git", "show", headRef}
			showErr  error
		)
		patch, showErr = exec.Execute(ctx, showArgs)
		if showErr != nil {
			return patch, fmt.Errorf("could not execute %v: %s after trying to execute %v: %v", showArgs, showErr, args, err)
		}
	}
	return patch, nil
}

// installAptPackages install packages using apt package manager, it expects
// apt-get update to have already been executed. Can be called with 0 or more
// packages.
func installAPTPackages(ctx context.Context, exec Executer, packages []string) error {
	if len(packages) == 0 {
		return nil
	}
	args := append([]string{"apt-get", "install", "-y"}, packages...)
	_, err := exec.Execute(ctx, args)
	return errors.Wrapf(err, "could not install %d apt_packages", len(packages))
}

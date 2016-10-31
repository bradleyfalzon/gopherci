package analyser

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/bradleyfalzon/gopherci/internal/db"
	"github.com/bradleyfalzon/revgrep"
	"github.com/pkg/errors"
)

// FileSystem analyses a repository and patch for issues using the file
// system. This is an insecure option and therefore should only be used when
// analysing a known safe repository with known safe static analysis tools.
//
// FileSystem is safe to use concurrently, as all directories are created
// with random file names.
type FileSystem struct {
	// gopath specifies the GOPATH to be set in the environment. Respositories
	// to be checked will be downloaded to $GOPATH/src/gopherci/, if the
	// repository directory already exists, it will be deleted.
	gopath string

	// copath specifies the base checkout path used, a temp folder name is created
	// within here to avoid race conditions with other threads.
	copath string

	// executer executes commands and other file system operations
	executer executer
}

// Ensure FileSystem implements Analyser
var _ Analyser = (*FileSystem)(nil)

func NewFileSystem(gopath string) (*FileSystem, error) {
	fs := &FileSystem{
		gopath:   gopath,
		executer: fsExecuter{},
	}

	// TODO check if gopath exists, and directory structure exists mkdirs if not
	// also check the ensure they are writable
	// $GOPATH/{src,pkg,bin}, $GOPATH/src/gopherci/

	return fs, nil
}

// Analyse implements Analyser interface
func (fs *FileSystem) Analyse(tools []db.Tool, repoURL, branch, diffURL string) ([]Issue, error) {
	log.Printf("fs.Analyse repoURL %q branch %q diffURL %q GOPATH %q", repoURL, branch, diffURL, fs.gopath)

	// download patch
	resp, err := http.Get(diffURL)
	if err != nil {
		return nil, err
	}
	patch, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, errors.Wrap(err, "could not ioutil.ReadAll response from"+diffURL)
	}

	// make temp dir
	tmpdir, err := fs.executer.Mktemp(filepath.Join(fs.gopath, "src", "gopherci"))
	if err != nil {
		return nil, err
	}

	// TODO on second thought, I was using tmpdir to allow safe concurrency
	// but go get isn't safe to run concurrently either. Perhaps it'll just be
	// better to either limit concurrency with some semaphore or create entire
	// gopaths separately.

	// clone repo
	// TODO check out https://godoc.org/golang.org/x/tools/go/vcs to be agnostic
	cmd := exec.Command("git", "clone", "--branch", branch, "--depth", "0", "--single-branch", repoURL, tmpdir)
	log.Printf("path: %v %v, dir: %v, env: %v", cmd.Path, cmd.Args, cmd.Dir, cmd.Env)
	out, err := fs.executer.CombinedOutput(cmd)
	if err != nil {
		return nil, fmt.Errorf("could not %v %v: %s\n%s", cmd.Path, cmd.Args, err, out)
	}
	//defer os.RemoveAll(tmpdir)

	log.Println("clone success to:", tmpdir)

	// fetch dependencies, some static analysis tools require building a project

	var issues []Issue
	for _, tool := range tools {
		cmd := exec.Command(tool.Path)
		cmd.Args = append(cmd.Args, strings.Fields(tool.Args)...)
		cmd.Env = []string{"GOPATH=" + fs.gopath, "PATH=" + os.Getenv("PATH")}
		cmd.Dir = tmpdir
		log.Printf("tool: %v, path: %v %v, dir: %v, env: %v", tool.Name, cmd.Path, cmd.Args, cmd.Dir, cmd.Env)
		// ignore errors, often it's about the exit status
		// TODO check these errors better, other static analysis tools check the code
		// explicitly or at least don't ignore it
		out, _ := fs.executer.CombinedOutput(cmd)
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

	return issues, nil
}

package analyser

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/pkg/errors"
)

// FileSystem is an Analyser than provides an Executer to build contained
// environments on the file system.
//
// FileSystem is safe to use concurrently, as all directories are created
// with random file names.
type FileSystem struct {
	base     string // base is the base dir all projects have in common
	memLimit int    // virtual memory limit in MiB for processes
}

// Ensure FileSystem implements Analyser
var _ Analyser = (*FileSystem)(nil)

// NewFileSystem returns an FileSystem which uses the path base to build
// contained environments on the file system. If memLimit is > 0, limit the
// amount of memory (MiB) a process can use.
func NewFileSystem(base string, memLimit int) (*FileSystem, error) {
	fs := &FileSystem{base: base, memLimit: memLimit}
	if err := unix.Access(base, unix.W_OK); err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("%q is not writable", base))
	}
	return fs, nil
}

// NewExecuter implements the Analyser interface
func (fs *FileSystem) NewExecuter(_ context.Context, goSrcPath string) (Executer, error) {
	e := &FileSystemExecuter{memLimit: fs.memLimit}
	if err := e.mktemp(fs.base, goSrcPath); err != nil {
		return nil, err
	}
	return e, nil
}

// FileSystemExecuter is an Executer that runs commands in a contained
// environment.
type FileSystemExecuter struct {
	gopath   string // gopath is base/$rand
	projpath string // projpath is gopath/src/<goSrcPath>
	memLimit int    // virtual memory limit in MiB for processes
}

// Ensure FileSystemExecuter implements Executer
var _ Executer = (*FileSystemExecuter)(nil)

func (e *FileSystemExecuter) mktemp(base, goSrcPath string) error {
	rand := strconv.Itoa(int(time.Now().UnixNano()))
	e.gopath = filepath.Join(base, rand)
	e.projpath = filepath.Join(e.gopath, "src", goSrcPath)

	if err := os.MkdirAll(e.projpath, 0700); err != nil {
		return errors.Wrap(err, "fsExecuter.Mktemp: cannot mkdir")
	}
	return nil
}

// Execute implements the Executer interface
func (e *FileSystemExecuter) Execute(ctx context.Context, args []string) ([]byte, error) {
	cmds := []string{
		fmt.Sprintf("ulimit -v %d", e.memLimit*1024),
		strings.Join(args, " "),
	}
	args = []string{"bash", "-c", strings.Join(cmds, " && ")}
	cmd := exec.CommandContext(ctx, "bash")
	cmd.Args = args
	cmd.Dir = e.projpath
	cmd.Env = []string{"GOPATH=" + e.gopath, "PATH=" + os.Getenv("PATH")}
	out, err := cmd.CombinedOutput()
	if msg, ok := err.(*exec.ExitError); ok {
		return out, &NonZeroError{ExitCode: msg.Sys().(syscall.WaitStatus).ExitStatus(), args: args}
	}
	return out, err
}

// Stop implements the Executer interface
func (e *FileSystemExecuter) Stop(_ context.Context) error {
	return os.RemoveAll(e.gopath)
}

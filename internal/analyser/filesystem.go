package analyser

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
	base string // base is the base dir all projects have in common
}

// Ensure FileSystem implements Analyser
var _ Analyser = (*FileSystem)(nil)

// NewFileSystem returns an FileSystem which uses the path base to build
// contained environments on the file system.
func NewFileSystem(base string) (*FileSystem, error) {
	fs := &FileSystem{base: base}
	if err := unix.Access(base, unix.W_OK); err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("%q is not writable", base))
	}
	return fs, nil
}

// NewExecuter implements the Analyser interface
func (fs *FileSystem) NewExecuter() (Executer, error) {
	return newFileSystemExecuter(fs.base)
}

// FileSystemExecuter is an Executer that runs commands in a contained
// environment.
type FileSystemExecuter struct {
	gopath   string // gopath is base/$rand
	projpath string // projpath is gopath/src/gopherci and stores the project
}

// Ensure FileSystemExecuter implements Executer
var _ Executer = (*FileSystemExecuter)(nil)

func newFileSystemExecuter(base string) (*FileSystemExecuter, error) {
	e := &FileSystemExecuter{}
	if err := e.mktemp(base); err != nil {
		return nil, err
	}
	return e, nil
}

func (e *FileSystemExecuter) mktemp(base string) error {
	rand := strconv.Itoa(int(time.Now().UnixNano()))
	e.gopath = filepath.Join(base, rand)
	e.projpath = filepath.Join(e.gopath, "src", "gopherci")

	if err := os.MkdirAll(e.projpath, 0700); err != nil {
		return errors.Wrap(err, "fsExecuter.Mktemp: cannot mkdir")
	}
	return nil
}

// Execute implements the Executer interface
func (e *FileSystemExecuter) Execute(args []string) ([]byte, error) {
	cmd := exec.Command(args[0])
	cmd.Args = args
	cmd.Dir = e.projpath
	cmd.Env = []string{"GOPATH=" + e.gopath, "PATH=" + os.Getenv("PATH")}
	return cmd.CombinedOutput()
}

// Stop implements the Executer interface
func (e *FileSystemExecuter) Stop() error {
	return os.RemoveAll(e.gopath)
}

package analyser

import (
	"os"
	"testing"
)

func TestNewFileSystem_notExist(t *testing.T) {
	base := "/does-not-exist"
	_, err := NewFileSystem(base)
	if err == nil {
		t.Errorf("expected error for path %v, got: %v", base, err)
	}
}

func TestFileSystem(t *testing.T) {
	fs, err := NewFileSystem(os.TempDir())
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	exec, err := fs.NewExecuter()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	gopath := exec.(*FileSystemExecuter).gopath

	if !exists(gopath) {
		t.Errorf("expected %q to exist", gopath)
	}

	out, err := exec.Execute([]string{"bash", "-c", "echo $GOPATH $PATH"})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if want := gopath + " " + os.Getenv("PATH") + "\n"; want != string(out) {
		t.Errorf("\nwant %s\nhave %s", want, out)
	}

	err = exec.Stop()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if exists(gopath) {
		t.Errorf("expected %q to be removed", gopath)
	}

}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil || !os.IsNotExist(err)
}

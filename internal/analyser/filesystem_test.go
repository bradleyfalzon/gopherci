package analyser

import (
	"context"
	"fmt"
	"os"
	"testing"
)

func TestNewFileSystem_notExist(t *testing.T) {
	memLimit := 512
	base := "/does-not-exist"
	_, err := NewFileSystem(base, memLimit)
	if err == nil {
		t.Errorf("expected error for path %v, got: %v", base, err)
	}
}

func TestFileSystem(t *testing.T) {
	memLimit := 512
	fs, err := NewFileSystem(os.TempDir(), memLimit)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	ctx := context.Background()

	exec, err := fs.NewExecuter(ctx, "github.com/gopherci/gopherci")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	gopath := exec.(*FileSystemExecuter).gopath

	if !exists(gopath) {
		t.Errorf("expected %q to exist", gopath)
	}

	out, err := exec.Execute(ctx, []string{"echo $GOPATH $PATH"})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if want := gopath + " " + os.Getenv("PATH") + "\n"; want != string(out) {
		t.Errorf("\nwant %s\nhave %s", want, out)
	}

	out, err = exec.Execute(ctx, []string{"pwd"})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Ensure current working directory is project path
	if want := gopath + "/src/github.com/gopherci/gopherci\n"; want != string(out) {
		t.Errorf("\nwant %q\nhave %q", want, out)
	}

	// Ensure correct memory limit
	out, err = exec.Execute(ctx, []string{"ulimit", "-v"})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if want := fmt.Sprintf("%d\n", memLimit*1024); want != string(out) {
		t.Errorf("\nwant %q\nhave %q", want, out)
	}

	err = exec.Stop(ctx)
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

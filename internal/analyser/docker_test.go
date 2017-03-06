package analyser

import (
	"log"
	"strings"
	"testing"
)

func TestDocker(t *testing.T) {
	docker, err := NewDocker(DockerDefaultImage)
	if err != nil {
		t.Fatalf("unexpected error initialising docker: %v", err)
	}

	exec, err := docker.NewExecuter("github.com/gopherci/gopherci")
	if err != nil {
		t.Fatalf("unexpected error in new executer: %v", err)
	}

	out, err := exec.Execute([]string{"pwd"})
	if err != nil {
		t.Errorf("unexpected error executing pwd: %v", err)
	}

	// Ensure current working directory is project path
	if want := "/go/src/github.com/gopherci/gopherci\n"; want != string(out) {
		t.Errorf("\nwant %q\nhave %q", want, out)
	}

	// Ensure error codes are captured
	out, err = exec.Execute([]string{">&2 echo error; false"})
	log.Printf("%q %q", string(out), err)
	if want := "error\n"; want != string(out) {
		t.Errorf("\nwant: %q\nhave: %q", want, out)
	}

	wantSuffix := "exit code 1"
	if !strings.HasSuffix(err.Error(), wantSuffix) {
		t.Errorf("\nwantSuffix: %q\nhave: %q", wantSuffix, err)
	}

	err = exec.Stop()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

package analyser

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	docker "github.com/fsouza/go-dockerclient"
	"github.com/pkg/errors"
)

const (
	stopContainerTimeout = 1
	// DockerDefaultImage defines the default docker image that can be used
	// to run checks.
	DockerDefaultImage = "gopherci/gopherci-env:latest"
)

// Docker is an Analyser that provides an Executer to build projects inside
// Docker containers.
type Docker struct {
	image  string
	client *docker.Client
}

// Ensure Docker implements Analyser interface.
var _ Analyser = (*Docker)(nil)

// NewDocker returns a Docker which uses imageName as a container to build
// projects.
func NewDocker(imageName string) (*Docker, error) {
	client, err := docker.NewClientFromEnv()
	if err != nil {
		return nil, err
	}

	info, err := client.Info()
	if err != nil {
		return nil, err
	}
	log.Printf("Docker server %q version %q on %q", info.Name, info.ServerVersion, info.OperatingSystem)

	// Check the image has been downloaded

	image, err := client.InspectImage(imageName)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("could not inspect %q", imageName))
	}
	log.Printf("Docker image %q (%v) created %v", imageName, image.ID, image.Created)

	return &Docker{image: imageName, client: client}, nil
}

// DockerExecuter is an Executer that runs commands in a contained
// environment for a single project.
type DockerExecuter struct {
	client    *docker.Client
	container *docker.Container
	projPath  string // path to project
}

// NewExecuter implements Analyser interface by creating and starting a
// docker container.
func (d *Docker) NewExecuter(ctx context.Context, goSrcPath string) (Executer, error) {
	exec := &DockerExecuter{
		client:   d.client,
		projPath: filepath.Join("$GOPATH", "src", goSrcPath),
	}

	name := fmt.Sprintf("goperci-%d", time.Now().UnixNano())

	createOptions := docker.CreateContainerOptions{
		Name:    name,
		Config:  &docker.Config{Image: d.image},
		Context: ctx,
	}

	// Create container
	var err error
	exec.container, err = d.client.CreateContainer(createOptions)
	if err != nil {
		return nil, errors.Wrap(err, "could not create container")
	}
	log.Printf("Created containerID %q named %q", exec.container.ID, name)

	// Start container
	if err := d.client.StartContainerWithContext(exec.container.ID, nil, ctx); err != nil {
		exec.Stop(ctx)
		return nil, errors.Wrap(err, "could not start container")
	}
	log.Printf("Started containerID %q", exec.container.ID)

	// Make required directories to clone into see bug in #16
	args := []string{"mkdir", "-p", exec.projPath}
	if out, err := exec.Execute(ctx, args); err != nil {
		exec.Stop(ctx)
		return nil, errors.Wrap(err, fmt.Sprintf("could not execute %v, output: %q", args, out))
	}

	return exec, nil
}

// Execute implements the Executer interface and runs commands inside a
// docker container.
func (e *DockerExecuter) Execute(ctx context.Context, args []string) ([]byte, error) {
	// "cd e.projPath; cmd" ignore the errors from cd as the first command
	// executed is the mkdir
	cmd := []string{"bash", "-c", fmt.Sprintf(`cd %v; %v`, e.projPath, strings.Join(args, " "))}
	createOptions := docker.CreateExecOptions{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          cmd,
		Container:    e.container.ID,
	}

	log.Printf("docker: creating exec for cmd: %v", cmd) // additional debug to troubleshoot unresponsive instance
	exec, err := e.client.CreateExec(createOptions)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("could not create exec for containerID %v", e.container.ID))
	}
	log.Printf("docker: created exec id: %v for cmd: %v", exec, cmd)

	var buf bytes.Buffer
	startOptions := docker.StartExecOptions{
		OutputStream: &buf,
		ErrorStream:  &buf,
		Context:      ctx,
	}

	// Start exec and block
	err = e.client.StartExec(exec.ID, startOptions)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("could not start exec, cmd: %v containerID %v", createOptions.Cmd, e.container.ID))
	}

	// Check error status of exec
	inspect, err := e.client.InspectExec(exec.ID)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("could not inspect exec for containerID %v", e.container.ID))
	}
	if inspect.ExitCode != 0 {
		return buf.Bytes(), &NonZeroError{ExitCode: inspect.ExitCode, args: args}
	}

	return buf.Bytes(), nil
}

// Stop stops and removes a container ignoring any errors.
func (e *DockerExecuter) Stop(ctx context.Context) error {
	err := e.client.StopContainerWithContext(e.container.ID, stopContainerTimeout, ctx)
	if err != nil {
		log.Printf("could not stop containerID %v: %v", e.container.ID, err)
		// Ignore the error and try to delete the container anyway
	}

	err = e.client.RemoveContainer(docker.RemoveContainerOptions{
		ID:            e.container.ID,
		RemoveVolumes: true,
		Force:         true,
		Context:       ctx,
	})
	if err != nil {
		log.Printf("could not remove containerID %v: %v", e.container.ID, err)
	}

	return nil
}

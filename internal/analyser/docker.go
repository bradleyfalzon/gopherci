package analyser

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/bradleyfalzon/gopherci/internal/logger"
	"github.com/fsouza/go-dockerclient"
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
	logger   logger.Logger
	image    string
	client   *docker.Client
	memLimit int // virtual memory limit in MiB for processes inside container (not container itself).
}

// Ensure Docker implements Analyser interface.
var _ Analyser = (*Docker)(nil)

// NewDocker returns a Docker which uses imageName as a container to build
// projects. If memLimit is > 0, limit the amount of memory (MiB) a process
// inside the container can use, this isn't a limit on the container itself.
func NewDocker(logger logger.Logger, imageName string, memLimit int) (*Docker, error) {
	client, err := docker.NewClientFromEnv()
	if err != nil {
		return nil, err
	}

	info, err := client.Info()
	if err != nil {
		return nil, err
	}
	logger.Infof("docker server %q version %q on %q", info.Name, info.ServerVersion, info.OperatingSystem)

	// Check the image has been downloaded

	image, err := client.InspectImage(imageName)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("could not inspect %q", imageName))
	}
	logger.Infof("docker image %q (%v) created %v", imageName, image.ID, image.Created)

	return &Docker{logger: logger, image: imageName, client: client, memLimit: memLimit}, nil
}

// DockerExecuter is an Executer that runs commands in a contained
// environment for a single project.
type DockerExecuter struct {
	logger    logger.Logger
	client    *docker.Client
	container *docker.Container
	projPath  string // path to project
	memLimit  int    // virtual memory limit in MiB for processes
}

// NewExecuter implements Analyser interface by creating and starting a
// docker container.
func (d *Docker) NewExecuter(ctx context.Context, goSrcPath string) (Executer, error) {
	exec := &DockerExecuter{
		logger:   d.logger,
		client:   d.client,
		projPath: filepath.Join("$GOPATH", "src", goSrcPath),
		memLimit: d.memLimit,
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
	exec.logger = d.logger.With("containerID", exec.container.ID)
	exec.logger.Info("created container")

	// Start container
	if err := d.client.StartContainerWithContext(exec.container.ID, nil, ctx); err != nil {
		exec.Stop(ctx)
		return nil, errors.Wrap(err, "could not start container")
	}
	exec.logger.Info("started container")

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
	cmds := []string{
		// Set memory limit for the running process.
		fmt.Sprintf("ulimit -v %d", e.memLimit*1024),
		// "cd e.projPath; cmd" ignore the errors from cd as the first command
		// executed is the mkdir.
		fmt.Sprintf("cd %v; %v", e.projPath, strings.Join(args, " ")),
	}

	cmd := []string{"bash", "-c", strings.Join(cmds, " && ")}
	createOptions := docker.CreateExecOptions{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          cmd,
		Container:    e.container.ID,
	}

	exec, err := e.client.CreateExec(createOptions)
	if err != nil {
		return nil, errors.Wrap(err, fmt.Sprintf("could not create exec for containerID %v", e.container.ID))
	}
	e.logger.Infof("created exec for cmd: %v", exec, cmd)

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
		e.logger.With("error", err).Error("could not stop container")
		// Ignore the error and try to delete the container anyway
	}

	err = e.client.RemoveContainer(docker.RemoveContainerOptions{
		ID:            e.container.ID,
		RemoveVolumes: true,
		Force:         true,
		Context:       ctx,
	})
	if err != nil {
		e.logger.With("error", err).Error("could not remove container")
	}

	return nil
}

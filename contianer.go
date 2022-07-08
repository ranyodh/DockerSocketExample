package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	log "github.com/sirupsen/logrus"
)

// ContainerExitCodeError is returned if RunContainerWithIO runs a container and
// it exits with a nonzero exit code.
type ContainerExitCodeError struct {
	ContainerName string
	ExitCode      int64
}

// Error gives a string description of ContainerExitCodeError.
func (e *ContainerExitCodeError) Error() string {
	return fmt.Sprintf("container %s exited with %d", e.ContainerName, e.ExitCode)
}

// containerAsync returns a handle to the running container. This handle can be used to cancel the container and check its output.
type containerAsync struct {
	client                client.CommonAPIClient
	runErr                error
	done                  bool
	waitContainerComplete chan struct{}
	waitSetupComplete     chan struct{}
	containerStop         chan struct{}
}

// Cancel cancels the container running asynchronously and waits for cleanup to occur. If the container already exited, does nothing.
func (c *containerAsync) Cancel() error {
	if c.done {
		// container is completed
		return nil
	}
	close(c.containerStop)
	<-c.waitContainerComplete
	return c.runErr
}

// Wait waits for the container run to complete
func (c *containerAsync) Wait() error {
	if c.done {
		// container is completed
		return nil
	}
	<-c.waitContainerComplete
	return c.runErr
}

// newcontainerAsync
func newcontainerAsync(client client.CommonAPIClient) *containerAsync {
	return &containerAsync{
		client:                client,
		waitContainerComplete: make(chan struct{}),
		waitSetupComplete:     make(chan struct{}),
		containerStop:         make(chan struct{}),
	}
}

// Done returns true iff the container is done running
func (c *containerAsync) Done() bool {
	return c.done
}

func asyncSetupComplete(canceller *containerAsync) {
	if canceller != nil {
		close(canceller.waitSetupComplete)
	}
}

var runContainerRetries = 60

// ContainerRunnable describes options to perform a container run
type ContainerRunnable struct {
	// required
	client     client.CommonAPIClient
	cfg        *container.Config
	hostConfig *container.HostConfig

	// optional
	name    string
	input   io.Reader
	stdout  io.Writer
	stderr  io.Writer
	timeout *time.Duration
	async   *containerAsync
}

func newContainerRunnable(client client.CommonAPIClient, cfg *container.Config, hostConfig *container.HostConfig) *ContainerRunnable {
	return &ContainerRunnable{
		client:     client,
		cfg:        cfg,
		hostConfig: hostConfig,
	}
}

// NewContainerRunnable creates a new container runnable on behalf of the caller. It requires a container and host config, but other arguments are passed optionally.
// Options are applied in order. The last option is applied last and thus takes precedence on other previous, potentially mutually exclusive option.
func NewContainerRunnable(client client.CommonAPIClient, cfg *container.Config, hostConfig *container.HostConfig, opts ...RunConfigOptionFunc) *ContainerRunnable {
	cr := newContainerRunnable(client, cfg, hostConfig)
	for _, applyOptionFunction := range opts {
		applyOptionFunction(cr)
	}
	return cr
}

// RunConfigOptionFunc is a function which configures an option for the container runnable
type RunConfigOptionFunc func(*ContainerRunnable)

// WithName adds a container name
func WithName(name string) RunConfigOptionFunc {
	return func(c *ContainerRunnable) {
		c.name = name
	}
}

// WithInput adds an input reader to be fed to the container
func WithInput(input io.Reader) RunConfigOptionFunc {
	return func(c *ContainerRunnable) {
		c.input = input
	}
}

// WithStdoutWriter adds an stdout writer
func WithStdoutWriter(stdout io.Writer) RunConfigOptionFunc {
	return func(c *ContainerRunnable) {
		c.stdout = stdout
	}
}

// WithStderrWriter adds an stderr writer
func WithStderrWriter(stderr io.Writer) RunConfigOptionFunc {
	return func(c *ContainerRunnable) {
		c.stderr = stderr
	}
}

// WithTimeout will add a timeout to the container run
func WithTimeout(timeout time.Duration) RunConfigOptionFunc {
	return func(c *ContainerRunnable) {
		c.timeout = &timeout
	}
}

// Run actually runs the container
func (cr *ContainerRunnable) Run(ctx context.Context) error {
	// handle timeout
	if cr.timeout != nil {
		var cancelFunc context.CancelFunc
		ctx, cancelFunc = context.WithTimeout(ctx, *cr.timeout)
		defer cancelFunc()
	}
	return runContainerWithIOBuffers(ctx, cr.client, cr.cfg, cr.hostConfig, cr.name, cr.input, cr.stdout, cr.stderr, cr.async)
}

// Start runs the container asynchronously. It does not return an error, as the error is captured by the Wait() call.
func (cr *ContainerRunnable) Start(ctx context.Context) {
	cr.async = newcontainerAsync(cr.client)
	go func(c *containerAsync) {
		c.runErr = cr.Run(ctx)
		c.done = true
		close(c.waitContainerComplete)
	}(cr.async)
	<-cr.async.waitSetupComplete
}

// Wait waits for sync
func (cr *ContainerRunnable) Wait() error {
	if cr.async == nil {
		// nothing to wait for
		return nil
	}
	return cr.async.Wait()
}

// Done returns true if the async job started is complete. If no async job exists, returns false
func (cr *ContainerRunnable) Done() bool {
	if cr.async == nil {
		// if no async
		return false
	}
	return cr.async.Done()
}

// Cancel will cancel the async job, if one has been started
func (cr *ContainerRunnable) Cancel() error {
	if cr.async == nil {
		return nil
	}
	return cr.async.Cancel()
}

// RunWithOutputBytes runs the containers and returns the stdout and stderr streams as bytes. Overrides any previous bytes option.
func (cr *ContainerRunnable) RunWithOutputBytes(ctx context.Context) ([]byte, []byte, error) {
	stdout := new(bytes.Buffer)
	stderr := new(bytes.Buffer)
	cr.stdout = stdout
	cr.stderr = stderr
	err := cr.Run(ctx)
	return stdout.Bytes(), stderr.Bytes(), err
}

// runContainerWithIOBuffers runs the given container config and saves the
// output to the given buffers.
func runContainerWithIOBuffers(ctx context.Context, client client.CommonAPIClient, cfg *container.Config, hostConfig *container.HostConfig, name string, input io.Reader, stdout, stderr io.Writer, canceller *containerAsync) error {
	resp, err := client.ContainerCreate(ctx, cfg, hostConfig, nil, nil, name)
	if err != nil {
		asyncSetupComplete(canceller)
		return fmt.Errorf("unable to create container %s: %s", name, err)
	}
	containerID := resp.ID
	if name == "" {
		// setting the name here to identify the container
		name = containerID
	}
	defer func() {
		removeErr := client.ContainerRemove(ctx, containerID, types.ContainerRemoveOptions{Force: true, RemoveVolumes: true})
		if removeErr != nil {
			log.Errorf("could not remove container %s: %s", containerID, removeErr)
		}
	}()

	var attachResp types.HijackedResponse
	attachResp, err = client.ContainerAttach(ctx, containerID, types.ContainerAttachOptions{
		Stream: true,
		Stdin:  input != nil,
		Stdout: true,
		Stderr: true,
	})
	if err != nil {
		asyncSetupComplete(canceller)
		return fmt.Errorf("unable to attach to container %s: %s", name, err)
	}

	if err = client.ContainerStart(ctx, containerID, types.ContainerStartOptions{
		CheckpointID: "",
	}); err != nil {
		asyncSetupComplete(canceller)
		return fmt.Errorf("unable to start container %s: %s", name, err)
	}
	timeout := 5 * time.Second
	defer func() {
		stopErr := client.ContainerStop(ctx, containerID, &timeout)
		if stopErr != nil {
			log.Errorf("could not stop container %s: %s", containerID, stopErr)
		}
	}()
	asyncSetupComplete(canceller)

	// This wait group will ensure that this function does not terminate until all spawned goroutines terminate as well
	var wg sync.WaitGroup
	defer func() {
		defer time.AfterFunc(5*time.Second, func() {
			// It's very unusual for the process of copying container output to
			// take longer than a moment after the container exists so we log
			// it as a warning.
			log.Warnf("Waiting for container %s to complete...", containerID)
		}).Stop()

		wg.Wait()
	}()

	// Wire this up for input
	if input != nil {
		// Send all remaining input to the second phase
		wg.Add(1)
		go func() {
			if _, err := io.Copy(attachResp.Conn, input); err != nil {
				log.Warnf("input copy interrupted: %s", err)
			}
			if err = attachResp.CloseWrite(); err != nil {
				log.Warnf("error closing input stream: %s", err)
			}
			wg.Done()
		}()
	}

	wg.Add(1)
	go func() {
		if cfg.Tty {
			if _, err := io.Copy(stdout, attachResp.Reader); err != nil {
				log.Errorf("TTY output copy interrupted: %s", err)
			}
		} else {
			if _, err = stdcopy.StdCopy(stdout, stderr, attachResp.Reader); err != nil {
				log.Errorf("Unable to read output from container %s: %s", name, err)
			}
		}
		wg.Done()
	}()

	var exitCode int64
	doneChan := make(chan int64)
	go func() {
		for i := 0; i < runContainerRetries; i++ {
			resC, errC := client.ContainerWait(ctx, containerID, container.WaitConditionNotRunning)
			select {
			case res := <-resC:
				exitCode = res.StatusCode
				err = nil
				doneChan <- exitCode
			case err = <-errC:
				exitCode = -1
				if strings.Contains(err.Error(), "Cannot connect to the Docker daemon") {
					time.Sleep(1 * time.Second)
					continue
				}
				if strings.Contains(err.Error(), "imeout exceeded while reading") {
					log.Debug("Encountered a timeout while waiting, will retry our ContainerWait")
					time.Sleep(1 * time.Second)
					continue
				}
				// Under some circumstances, our tests can bounce the swarm manager through
				// which we were running the ContainerWait, which results in an EOF
				if strings.Contains(err.Error(), "EOF") {
					log.Debug("Encountered an EOF while waiting, will retry our ContainerWait")
					// Wait a little longer before hitting the API again so classic swarm can re-elect
					time.Sleep(30 * time.Second)
					continue
				}
			}
			break // Either got an exitCode or an unexpected error.
		}
	}()

	log.Debugf("waiting for the container '%s' to complete", name)
	var interrupt chan struct{}
	if canceller != nil {
		interrupt = canceller.containerStop
	} else {
		// if not interrupting, use a channel which will never receive a message. Other cases will be selected instead
		unusedChan := make(chan struct{})
		interrupt = unusedChan
	}
	select {
	case <-interrupt:
		log.Infof("container %s interrupted", name)
		return nil
	case <-ctx.Done():
		return fmt.Errorf("could not wait for container '%s' to finish: %s", name, ctx.Err())
	case exitCode := <-doneChan:
		if err != nil {
			err = fmt.Errorf("unable to wait for container %s: %s", name, err)
		} else if exitCode == 0 {
			err = nil
		} else {
			err = &ContainerExitCodeError{ContainerName: name, ExitCode: exitCode}
		}
		return err
	}
}

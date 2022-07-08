package main

import (
	"context"
	"fmt"
	"os"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	log "github.com/sirupsen/logrus"
)

// DockerAPILocalPath is the location of the Docker daemon Unix socket.
const DockerAPILocalPath = `/var/run/docker.sock`
const ContainerToRun = "hello-world"

func main() {

	client, err := NewDockerClient()
	if err != nil {
		log.Errorf("error while creating docker client: %v", err)
		os.Exit(1)
	}

	ctx := context.Background()

	if err := runContainer(ctx, client, ContainerToRun); err != nil {
		log.Errorf("error while running container: %v", err)
		os.Exit(1)
	}

	os.Exit(0)
}

func NewDockerClient() (client.CommonAPIClient, error) {
	dockerClient, err := client.NewClientWithOpts(
		client.WithHost(fmt.Sprintf("unix://%s", DockerAPILocalPath)),
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, fmt.Errorf("could not create a Docker client: %s", err)
	}
	return dockerClient, nil
}

func runContainer(ctx context.Context, client client.CommonAPIClient, image string) error {

	err := PullImage(ctx, client, image)
	if err != nil {
		return fmt.Errorf("failed to pull image '%s': %v", image, err)
	}

	containerConfig := &container.Config{
		Image:        image,
		Tty:          false,
		OpenStdin:    true,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		StdinOnce:    true,
	}

	hostConfig := &container.HostConfig{
		Mounts: []mount.Mount{
			{
				Type:   mount.TypeBind,
				Source: DockerAPILocalPath,
				Target: DockerAPILocalPath,
			},
		},
	}

	log.Info("Running install agent container ...")
	return NewContainerRunnable(
		client, containerConfig, hostConfig,
		WithName("hello-world"),
		WithStdoutWriter(os.Stdout),
		WithStderrWriter(os.Stderr),
	).Run(ctx)
}

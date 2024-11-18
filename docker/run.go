package docker

import (
	"context"
	"fmt"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/errdefs"
	"github.com/docker/go-connections/nat"
	"io"
	"log/slog"
	"paas/domain"
	"strconv"
	"strings"
)

type RunOptions struct {
	Stdout io.WriteCloser
	// If we should kill the existing container that's running first
	RemoveExisting bool
	// Whether we should just return if the container is already running
	IgnoreIfRunning bool
}

func (c *Client) GetContainer(resource *domain.Resource, index int) (types.ContainerJSON, error) {
	containerName := fmt.Sprintf("%s-%s-container-%d", resource.Name, resource.Id, index)
	return c.cli.ContainerInspect(context.Background(), containerName)
}

func (c *Client) Stop(resource *domain.Resource) error {
	for i := range resource.InstancesPerServer {
		containerName := fmt.Sprintf("%s-%s-container-%d", resource.Name, resource.Id, i)
		err := c.cli.ContainerStop(context.Background(), containerName, container.StopOptions{})
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) Run(resource *domain.Resource, opts RunOptions) error {
	instances := resource.InstancesPerServer
	if instances == 0 {
		instances = 1
	}

	c.ReduceToMatchResourceCount(resource, instances)

	for i := range instances {
		err := c.doRun(resource, i, opts)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) doRun(resource *domain.Resource, index int, opts RunOptions) error {
	ctx := context.Background()
	imageName := fmt.Sprintf("%s-%s", resource.Name, resource.Id)
	containerName := fmt.Sprintf("%s-%s-container-%d", resource.Name, resource.Id, index)

	if opts.IgnoreIfRunning {
		exists, err := c.GetContainer(resource, index)
		// if the container exists and is running, we can just return
		if err == nil && exists.State.Running {
			return nil
		}
	}

	err := c.cli.ContainerStop(ctx, containerName, container.StopOptions{})

	if err != nil {
		switch err.(type) {
		case errdefs.ErrNotFound:
			// don't need to worry about it if the container doesn't exist
			err = nil
		default:
			return err
		}
	}

	if opts.RemoveExisting {
		err = c.cli.ContainerRemove(ctx, containerName, container.RemoveOptions{
			Force: true,
		})
	}

	if err != nil {
		switch err.(type) {
		case errdefs.ErrNotFound:
			// don't need to worry about it if the container doesn't exist
			err = nil
		default:
			return err
		}
	}

	hostPort, err := FindOpenPort(3000)

	if err != nil {
		return err
	}

	// Define port bindings
	portBindings := nat.PortMap{
		"3000/tcp": []nat.PortBinding{
			{
				HostIP:   "0.0.0.0",              // Bind to all network interfaces
				HostPort: strconv.Itoa(hostPort), // Map container port 80 to host port 8080
			},
		},
	}

	hostConfig := &container.HostConfig{
		PortBindings: portBindings,
		LogConfig: container.LogConfig{
			Type: "json-file",
			Config: map[string]string{
				"max-size": "10m",
			},
		},
	}

	// Create and start a container
	resp, err := c.cli.ContainerCreate(ctx, &container.Config{
		Image: imageName,
		ExposedPorts: map[nat.Port]struct{}{
			// the port the container exposes
			// TODO this should be dynamic
			"3000/tcp": {},
		},
		AttachStdout: true,
		AttachStderr: true,
	}, hostConfig, nil, nil, containerName)

	if err != nil {
		switch err.(type) {
		case errdefs.ErrNotFound:
			return domain.ResourceNotFoundError
		case errdefs.ErrConflict:
			// container already exists, it failed to get killed for some reason
			if opts.RemoveExisting {
				return domain.ContainerExistsError
			} else {
				// we don't want to remove existing, so lets run the current one
				err = nil
			}
		default:
			return err
		}
	}

	err = c.cli.ContainerStart(ctx, containerName, container.StartOptions{})

	if err != nil {
		// another container may have taken the port, lets try a different one
		if strings.Contains(err.Error(), "port is already allocated") {
			slog.Error("Port is already allocated, trying a different one", slog.String("container_name", containerName))
			for i := 0; i < 50; i++ {
				err = c.doRun(resource, index, opts)
				if err == nil {
					return nil
				}
			}
		}
		// the port this container is trying to bind to is already in use
		// this can happen if we reboot the container and something else took it
		// kind of edge case, but it can happen, ideally we should be able to kill the container
		// and start it again, but we can't do that if opts.RemoveExisting is false
		if strings.Contains(err.Error(), "address already in use") {
			return domain.ResourcePortInUseError(strconv.Itoa(hostPort))
		}

		return err
	}

	if opts.Stdout != nil {
		return c.StreamLogs(resp.ID, ctx, StreamLogsOptions{
			Stdout: opts.Stdout,
		})
	}

	return nil
}

package docker

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types/container"
)

// PerformAction executes a lifecycle command on a specific container
func (e *Engine) PerformAction(ctx context.Context, containerID string, action string) error {
	if e.Client == nil {
		return fmt.Errorf("docker engine is not connected")
	}

	switch action {
	case "start":
		return e.Client.ContainerStart(ctx, containerID, container.StartOptions{})
	case "stop":
		// Using nil timeout uses the graceful default stop timeout (10s) before killing it
		return e.Client.ContainerStop(ctx, containerID, container.StopOptions{})
	case "restart":
		return e.Client.ContainerRestart(ctx, containerID, container.StopOptions{})
	case "delete":
		// Force: true kills it if it's running.
		// RemoveVolumes: false ensures we don't accidentally wipe persistent app data!
		return e.Client.ContainerRemove(ctx, containerID, container.RemoveOptions{
			Force:         true,
			RemoveVolumes: false,
		})
	default:
		return fmt.Errorf("unknown action: %s", action)
	}
}

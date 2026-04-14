package docker

import (
	"context"
	"fmt"
	"strings"

	"github.com/docker/docker/api/types/container"
)

// ContainerInfo is our clean, human-readable structure for the UI
type ContainerInfo struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Image string   `json:"image"`
	State string   `json:"state"` // "running", "exited", "restarting"
	Ports []string `json:"ports"`
}

// ListContainers asks the Docker daemon for all containers (both running and stopped)
func (e *Engine) ListContainers(ctx context.Context) ([]ContainerInfo, error) {
	if e.Client == nil {
		return nil, fmt.Errorf("docker engine is not connected")
	}

	// Fetch all containers (All: true means we see stopped ones too)
	containers, err := e.Client.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %v", err)
	}

	var results []ContainerInfo

	for _, c := range containers {
		// Docker names always start with a slash (e.g., "/gitea"). We clean it up here.
		name := "unknown"
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}

		// Format the ports nicely (e.g., "8080->80/tcp")
		var portList []string
		for _, p := range c.Ports {
			if p.PublicPort != 0 {
				portList = append(portList, fmt.Sprintf("%d->%d/%s", p.PublicPort, p.PrivatePort, p.Type))
			} else {
				portList = append(portList, fmt.Sprintf("%d/%s", p.PrivatePort, p.Type))
			}
		}

		results = append(results, ContainerInfo{
			ID:    c.ID[:12], // We only need the short ID for the UI
			Name:  name,
			Image: c.Image,
			State: c.State,
			Ports: portList,
		})
	}

	return results, nil
}

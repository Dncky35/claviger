package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"
)

// GetLogs fetches the last 100 lines of a container's output cleanly
func (e *Engine) GetLogs(ctx context.Context, containerID string) (string, error) {
	if e.Client == nil {
		return "", fmt.Errorf("docker engine is not connected")
	}

	// We only want the last 100 lines so we don't crash the browser with massive text walls
	options := container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       "100",
	}

	out, err := e.Client.ContainerLogs(ctx, containerID, options)
	if err != nil {
		return "", err
	}
	defer out.Close()

	// Docker mixes stdout and stderr with invisible headers. stdcopy separates them cleanly!
	var stdoutBuf, stderrBuf bytes.Buffer
	_, err = stdcopy.StdCopy(&stdoutBuf, &stderrBuf, out)
	if err != nil {
		return "", err
	}

	// Combine them for the UI, with standard output first, then errors
	finalLogs := stdoutBuf.String() + stderrBuf.String()
	return finalLogs, nil
}

// GetStats fetches a single snapshot of the container's CPU and Memory usage
func (e *Engine) GetStats(ctx context.Context, containerID string) (map[string]interface{}, error) {
	if e.Client == nil {
		return nil, fmt.Errorf("docker engine is not connected")
	}

	// stream: false means we just want one snapshot right now, not a continuous websocket
	statsRaw, err := e.Client.ContainerStats(ctx, containerID, false)
	if err != nil {
		return nil, err
	}
	defer statsRaw.Body.Close()

	// Decode Docker's massive raw JSON response directly into a Go map
	var statsData map[string]interface{}
	bodyBytes, _ := io.ReadAll(statsRaw.Body)
	if err := json.Unmarshal(bodyBytes, &statsData); err != nil {
		return nil, fmt.Errorf("failed to parse stats JSON: %v", err)
	}

	return statsData, nil
}

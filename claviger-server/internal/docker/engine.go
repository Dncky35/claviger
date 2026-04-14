package docker

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/docker/docker/client"
)

// Engine holds the active connection to the host's Docker daemon
type Engine struct {
	Client *client.Client
}

// NewEngine initializes the Docker SDK and verifies the daemon is responding
func NewEngine() (*Engine, error) {
	log.Println("🐳 Initializing Docker Orchestration Engine...")

	// client.FromEnv automatically looks for the local /var/run/docker.sock
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Docker client: %v", err)
	}

	// Create a quick 5-second timeout context just for the initial Ping
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Ping the daemon to prove it's actually running
	ping, err := cli.Ping(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not ping Docker daemon (is it running?): %v", err)
	}

	log.Printf("✅ Docker Engine connected! (API Version: %s)", ping.APIVersion)

	return &Engine{
		Client: cli,
	}, nil
}

// Close cleanly shuts down the Docker client connection
func (e *Engine) Close() {
	if e.Client != nil {
		e.Client.Close()
	}
}

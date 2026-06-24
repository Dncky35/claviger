package docker

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
)

// Engine holds the active connection to the host's Docker daemon
type Engine struct {
	Client *client.Client
}

// NewEngine initializes the Docker SDK, verifies the daemon, and ensures core infrastructure exists
func NewEngine() (*Engine, error) {
	log.Println("🐳 Initializing Docker Orchestration Engine...")

	// client.FromEnv automatically looks for the local /var/run/docker.sock
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Docker client: %v", err)
	}

	// Create a quick 5-second timeout context just for the initial boot sequence
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Ping the daemon to prove it's actually running
	ping, err := cli.Ping(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not ping Docker daemon (is it running?): %v", err)
	}

	log.Printf("✅ Docker Engine connected! (API Version: %s)", ping.APIVersion)

	engine := &Engine{
		Client: cli,
	}

	// 🛡️ ZERO TRUST ORCHESTRATION: Ensure the master bridge network exists
	if err := engine.EnsureNetworkExists(ctx, "cloudrocean-net"); err != nil {
		return nil, fmt.Errorf("critical network initialization failed: %v", err)
	}

	return engine, nil
}

// EnsureNetworkExists uses the Docker SDK to check for the core network and creates it if missing
func (e *Engine) EnsureNetworkExists(ctx context.Context, networkName string) error {
	// Create a filter to search specifically for our network name
	netFilter := filters.NewArgs()
	netFilter.Add("name", "^"+networkName+"$") // Regex exact match

	// Query the Docker socket
	networks, err := e.Client.NetworkList(ctx, network.ListOptions{Filters: netFilter})
	if err != nil {
		return fmt.Errorf("failed to list docker networks: %v", err)
	}

	// If the network exists, we are good to go
	if len(networks) > 0 {
		log.Printf("🌐 Core network '%s' verified.", networkName)
		return nil
	}

	log.Printf("🏗️ Core network '%s' not found. Provisioning native bridge network...", networkName)

	// Network doesn't exist, tell Docker to create it natively
	_, err = e.Client.NetworkCreate(ctx, networkName, network.CreateOptions{
		Driver: "bridge",
		// You can add internal: false or specific subnet configs here in the future if needed
	})

	if err != nil {
		return fmt.Errorf("failed to create network %s: %v", networkName, err)
	}

	log.Printf("✅ Core network '%s' successfully provisioned!", networkName)
	return nil
}

// Close cleanly shuts down the Docker client connection
func (e *Engine) Close() {
	if e.Client != nil {
		e.Client.Close()
	}
}

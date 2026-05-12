package apps

import (
	"fmt"
	"net"
	"strconv"
	"time"
)

// IsPortAvailable checks if a specific port and protocol (tcp/udp) is free on the host.
func IsPortAvailable(port int, protocol string) bool {
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))

	// Set a quick timeout so the UI doesn't hang if the network stack is slow
	timeout := 1 * time.Second

	switch protocol {
	case "tcp":
		conn, err := net.DialTimeout("tcp", address, timeout)
		if err != nil {
			// Connection refused/timeout means nothing is listening. The port is free!
			return true
		}
		if conn != nil {
			conn.Close()
			return false // Someone answered, port is busy.
		}
	case "udp":
		// UDP is connectionless, so we use ListenUDP to see if we can bind to it
		addr, err := net.ResolveUDPAddr("udp", address)
		if err != nil {
			return false
		}
		conn, err := net.ListenUDP("udp", addr)
		if err != nil {
			return false // Address already in use
		}
		conn.Close()
		return true
	}

	return false
}

// GetNextAvailablePort scans a block of ports (e.g., 18081 to 18099)
// and returns the first completely free port it finds.
func GetNextAvailablePort(startPort int, maxPort int) (int, error) {
	for port := startPort; port <= maxPort; port++ {
		// For web apps, we primarily care about TCP
		if IsPortAvailable(port, "tcp") {
			return port, nil
		}
	}
	return 0, fmt.Errorf("no available ports found in the range %d - %d", startPort, maxPort)
}

// VerifyAppPorts acts as a pre-flight check before running docker-compose
func VerifyAppPorts(ports []int) error {
	for _, port := range ports {
		if !IsPortAvailable(port, "tcp") {
			return fmt.Errorf("port %d is already in use by another service on the host machine", port)
		}
	}
	return nil
}

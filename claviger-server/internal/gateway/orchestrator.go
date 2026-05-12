package gateway

import (
	"fmt"
	"net"
	"os/exec"
	"strings"
)

// ==========================================
// 1. THE DETECT PHASE (Safety First)
// ==========================================

// CheckPorts ensures the host machine has ports 80 and 443 available.
// We must run this BEFORE allowing the catalog to install "npm".
func CheckPorts() error {
	ports := []string{"80", "443"}

	for _, port := range ports {
		// Try to listen on the port
		ln, err := net.Listen("tcp", ":"+port)
		if err != nil {
			return fmt.Errorf("Port %s is currently in use. Please stop the service holding this port (e.g., your existing Cloudrocean Nginx container)", port)
		}
		ln.Close() // It's free, release it immediately
	}
	return nil
}

// ==========================================
// 2. STATUS CHECK (For your Hub UI)
// ==========================================

// IsGatewayRunning checks if the NPM container is currently active
func IsGatewayRunning() bool {
	// 🎯 UPDATE THIS TO LOOK FOR name=npm
	cmd := exec.Command("docker", "ps", "-q", "-f", "name=npm")
	out, err := cmd.Output()
	if err != nil || len(strings.TrimSpace(string(out))) == 0 {
		return false
	}
	return true
}

package firewall

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
)

// =========================================================
// SYSCTL MANAGEMENT (Persistent IP Forwarding)
// =========================================================

func enablePersistentForwarding() error {
	filePath := "/etc/sysctl.conf"
	contentBytes, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	content := string(contentBytes)

	if strings.Contains(content, "BEGIN CLAVIGER SYSCTL") {
		return nil // Already injected
	}

	block := "\n# --- BEGIN CLAVIGER SYSCTL ---\nnet.ipv4.ip_forward=1\n# --- END CLAVIGER SYSCTL ---\n"

	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.WriteString(block); err != nil {
		return err
	}

	// Apply immediately
	exec.Command("sysctl", "-p").Run()
	return nil
}

func disablePersistentForwarding() error {
	filePath := "/etc/sysctl.conf"
	contentBytes, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	content := string(contentBytes)

	if !strings.Contains(content, "BEGIN CLAVIGER SYSCTL") {
		return nil
	}

	startIdx := strings.Index(content, "# --- BEGIN CLAVIGER SYSCTL ---")
	endTag := "# --- END CLAVIGER SYSCTL ---\n"
	endIdx := strings.Index(content, endTag)

	if startIdx != -1 && endIdx != -1 {
		newContent := content[:startIdx] + content[endIdx+len(endTag):]
		os.WriteFile(filePath, []byte(newContent), 0644)
	}

	return nil
}

// =========================================================
// FIREWALL LIFECYCLE
// =========================================================

// SetupFirewall configures the baseline UFW rules and persistent kernel routing.
func SetupFirewall(port string) error {
	fmt.Println("🛡️  Configuring baseline firewall & kernel routing...")

	if _, err := exec.LookPath("ufw"); err != nil {
		log.Printf("⚠️ Warning: UFW not found, skipping firewall configuration: %v", err)
		return fmt.Errorf("UFW not found: %v", err)
	}

	if err := enablePersistentForwarding(); err != nil {
		log.Printf("⚠️ Warning: Failed to set persistent IP forwarding: %v", err)
		return fmt.Errorf("failed to set persistent IP forwarding: %v", err)
	}

	cmds := [][]string{
		// 1. The Front Door: Allow encrypted WireGuard traffic from the public internet
		{"ufw", "allow", port + "/udp"},

		// 2. The Private LAN (The Big Umbrella):
		// Allow VPN clients to reach Docker containers, the Hub, and SSH (Port 22).
		// (Standard users will hit the Go Middleware and be rejected by your Hub app!)
		{"ufw", "allow", "in", "on", "wg0", "to", "any"},
	}

	for _, args := range cmds {
		exec.Command(args[0], args[1:]...).Run()
	}
	exec.Command("ufw", "reload").Run()
	fmt.Println("   [✓] Baseline Firewall configured.")
	return nil
}

// TeardownFirewall removes the Claviger baseline rules and restores the kernel.
func TeardownFirewall(port string) error {
	fmt.Println("🧹 Removing Claviger firewall rules...")

	if _, err := exec.LookPath("ufw"); err != nil {
		return fmt.Errorf("UFW not found: %v", err)
	}

	disablePersistentForwarding()

	cmds := [][]string{
		{"ufw", "delete", "allow", port + "/udp"},
		{"ufw", "delete", "allow", "in", "on", "wg0", "to", "any"},
	}

	for _, args := range cmds {
		exec.Command(args[0], args[1:]...).Run()
	}
	exec.Command("ufw", "reload").Run()
	fmt.Println("   [✓] Firewall restored to pre-install state.")

	return nil
}

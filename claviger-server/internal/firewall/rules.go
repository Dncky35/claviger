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
func SetupFirewall() {
	fmt.Println("🛡️  Configuring baseline firewall & kernel routing...")

	if err := enablePersistentForwarding(); err != nil {
		log.Printf("⚠️ Warning: Failed to set persistent IP forwarding: %v", err)
	}

	cmds := [][]string{
		{"ufw", "allow", "51820/udp"},                                 // Allow WireGuard port
		{"ufw", "allow", "in", "on", "wg0", "to", "any"},              // Allow traffic from wg0 interface
		{"ufw", "route", "allow", "from", "10.8.0.0/24", "to", "any"}, // Allow VPN subnet to route
	}

	for _, args := range cmds {
		exec.Command(args[0], args[1:]...).Run()
	}
	exec.Command("ufw", "reload").Run()
	fmt.Println("   [✓] Baseline Firewall configured.")
}

// TeardownFirewall removes the Claviger baseline rules and restores the kernel.
func TeardownFirewall() {
	fmt.Println("🧹 Removing Claviger firewall rules...")

	disablePersistentForwarding()

	cmds := [][]string{
		{"ufw", "delete", "allow", "51820/udp"},
		{"ufw", "delete", "allow", "in", "on", "wg0", "to", "any"},
		{"ufw", "route", "delete", "allow", "from", "10.8.0.0/24", "to", "any"},
	}

	for _, args := range cmds {
		exec.Command(args[0], args[1:]...).Run()
	}
	exec.Command("ufw", "reload").Run()
	fmt.Println("   [✓] Firewall restored to pre-install state.")
}

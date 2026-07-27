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

	// 1. Read file, but don't fail if it simply doesn't exist yet
	contentBytes, err := os.ReadFile(filePath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read %s: %v", filePath, err)
	}
	content := string(contentBytes)

	// 2. Check if already injected
	if strings.Contains(content, "BEGIN CLAVIGER SYSCTL") {
		// Ensure it's active in the live kernel session even if the file was already correct
		exec.Command("sysctl", "-w", "net.ipv4.ip_forward=1").Run()
		return nil
	}

	block := "\n# --- BEGIN CLAVIGER SYSCTL ---\nnet.ipv4.ip_forward=1\n# --- END CLAVIGER SYSCTL ---\n"

	// 3. Open with O_CREATE to handle fresh/minimal servers gracefully
	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return fmt.Errorf("failed to open/create %s: %v", filePath, err)
	}
	defer f.Close()

	if _, err := f.WriteString(block); err != nil {
		return fmt.Errorf("failed to write block to %s: %v", filePath, err)
	}

	// 4. Apply immediately to the live kernel AND reload the file
	exec.Command("sysctl", "-w", "net.ipv4.ip_forward=1").Run()
	exec.Command("sysctl", "-p").Run()

	return nil
}

func disablePersistentForwarding() error {
	filePath := "/etc/sysctl.conf"

	// 1. If file doesn't exist, there is nothing to clean up
	contentBytes, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read %s: %v", filePath, err)
	}
	content := string(contentBytes)

	// 2. Look for our block
	if !strings.Contains(content, "BEGIN CLAVIGER SYSCTL") {
		return nil
	}

	startIdx := strings.Index(content, "# --- BEGIN CLAVIGER SYSCTL ---")
	endTag := "# --- END CLAVIGER SYSCTL ---\n"
	endIdx := strings.Index(content, endTag)

	// 3. Safely slice out the block
	if startIdx != -1 && endIdx != -1 {
		// Include a check to remove the preceding newline if it exists so we don't leave empty lines
		if startIdx > 0 && content[startIdx-1] == '\n' {
			startIdx--
		}

		newContent := content[:startIdx] + content[endIdx+len(endTag):]

		// Write the cleaned file back with standard 0644 permissions
		if err := os.WriteFile(filePath, []byte(newContent), 0644); err != nil {
			return fmt.Errorf("failed to update %s: %v", filePath, err)
		}
	}

	// 4. Disable forwarding in the live kernel instantly
	// (Note: sysctl -p won't turn it off just because we removed it from the file)
	exec.Command("sysctl", "-w", "net.ipv4.ip_forward=0").Run()

	return nil
}

// =========================================================
// FIREWALL LIFECYCLE
// =========================================================

// SetupFirewall configures the baseline UFW rules and persistent kernel routing.
func SetupFirewall(port string, sshLockdown bool) error {
	fmt.Println("🛡️  Configuring baseline firewall & kernel routing...")

	if _, err := exec.LookPath("ufw"); err != nil {
		log.Printf("⚠️ Warning: UFW not found, skipping firewall configuration: %v", err)
		return fmt.Errorf("UFW not found: %v", err)
	}

	if err := enablePersistentForwarding(); err != nil {
		log.Printf("⚠️ Warning: Failed to set persistent IP forwarding: %v", err)
		return fmt.Errorf("failed to set persistent IP forwarding: %v", err)
	}

	// Baseline rules that ALWAYS apply
	cmds := [][]string{
		// 1. The Front Door: Allow encrypted WireGuard traffic from the public internet
		{"ufw", "allow", port + "/udp"},

		// 2. The Private LAN (The Big Umbrella):
		// Allow VPN clients to reach Docker containers, the Hub, and SSH (Port 22).
		{"ufw", "allow", "in", "on", "wg0", "to", "any"},
	}

	// 3. Conditional SSH Rule based on DB State
	if sshLockdown {
		fmt.Println("   [🔒] SSH Lockdown is ACTIVE. Bypassing public Port 22 rule.")
		cmds = append(cmds, []string{"ufw", "delete", "allow", "22/tcp"})
		cmds = append(cmds, []string{"ufw", "delete", "allow", "22"})
	} else {
		fmt.Println("   [🔓] SSH Lockdown is OFF. Ensuring public Port 22 access...")
		cmds = append(cmds, []string{"ufw", "allow", "22/tcp"})
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

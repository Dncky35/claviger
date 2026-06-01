package network

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// InstallWireGuard checks if WireGuard is installed, and if not, installs it via apt.
func InstallWireGuard() error {
	if runtime.GOOS != "linux" {
		log.Println("⚠️ Skipping WireGuard install: Operating system is not Linux.")
		return nil
	}

	_, err := exec.LookPath("wg")
	if err == nil {
		log.Println("✅ WireGuard is already installed on this system.")
		return nil
	}

	log.Println("⚙️ WireGuard not found. Installing via apt-get...")
	updateCmd := exec.Command("apt-get", "update")
	if err := updateCmd.Run(); err != nil {
		return fmt.Errorf("failed to update apt repositories: %v", err)
	}

	installCmd := exec.Command("apt-get", "install", "-y", "wireguard", "wireguard-tools")
	installCmd.Stdout = os.Stdout
	installCmd.Stderr = os.Stderr
	if err := installCmd.Run(); err != nil {
		return fmt.Errorf("failed to install wireguard: %v", err)
	}

	log.Println("✅ WireGuard installed successfully.")
	return nil
}

// GenerateKeys creates a secure Private/Public key pair for the server
func GenerateKeys() (privateKey string, publicKey string, err error) {
	key, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		return "", "", err
	}
	return key.String(), key.PublicKey().String(), nil
}

// WriteBaseConfig writes a clean wg0.conf file.
// It DOES NOT include peers or NAT rules. SQLite and the Go daemon handle those dynamically.
func WriteBaseConfig(privateKey string, listenPort string) error {
	if runtime.GOOS != "linux" {
		return nil // Skip file creation on Windows/Mac testing
	}

	// 1. Ensure IP forwarding is on at the kernel level
	if err := EnableIPForwarding(); err != nil {
		log.Printf("⚠️ Warning: %v", err)
	}

	configPath := "/etc/wireguard/wg0.conf"
	os.MkdirAll("/etc/wireguard", 0700)

	// SaveConfig is FALSE so WireGuard doesn't overwrite our SQLite memory state
	configContent := fmt.Sprintf(`[Interface]
PrivateKey = %s
Address = 10.8.0.1/24
ListenPort = %s
SaveConfig = false

# ===================================================================
# DO NOT ADD PEERS HERE.
# The Claviger daemon manages peers and firewall rules dynamically 
# in the Linux Kernel using wgctrl and SQLite.
# ===================================================================
`, privateKey, listenPort)

	err := os.WriteFile(configPath, []byte(configContent), 0600)
	if err != nil {
		return fmt.Errorf("failed to write wg0.conf: %v", err)
	}

	log.Println("✅ WireGuard base configuration (wg0.conf) created successfully.")
	return nil
}

// GetPublicIP automatically finds the server's public IP address
func GetPublicIP() (string, error) {
	client := http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("https://api.ipify.org")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	ipBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(ipBytes), nil
}

// GetPeerCounts queries the kernel to see how many peers are active (Handshake < 3 mins)
func GetPeerCounts() (total int, active int) {
	if runtime.GOOS != "linux" {
		return 0, 0
	}

	client, err := wgctrl.New()
	if err != nil {
		return 0, 0
	}
	defer client.Close()

	device, err := client.Device("wg0")
	if err != nil {
		return 0, 0
	}

	totalCount := len(device.Peers)
	activeCount := 0

	for _, peer := range device.Peers {
		if !peer.LastHandshakeTime.IsZero() && time.Since(peer.LastHandshakeTime) < 3*time.Minute {
			activeCount++
		}
	}

	return totalCount, activeCount
}

// CheckAndOpenFirewall ensures the custom WireGuard port is open if UFW is active.
func CheckAndOpenFirewall(port string) error {
	if runtime.GOOS != "linux" {
		return nil
	}

	// 1. Explicitly check if UFW is installed on the system
	_, err := exec.LookPath("ufw")
	if err != nil {
		log.Println("ℹ️  UFW firewall is not installed on this system. Skipping firewall configuration.")
		return nil
	}

	// 2. Check if UFW is active
	cmd := exec.Command("ufw", "status")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("⚠️  UFW is installed, but status check failed (are you running as root?): %v", err)
		return nil
	}

	// 3. If active, open the required port
	if strings.Contains(string(output), "Status: active") {
		log.Printf("🛡️  UFW is active. Opening WireGuard port %s/udp...", port)

		allowCmd := exec.Command("ufw", "allow", fmt.Sprintf("%s/udp", port))
		if err := allowCmd.Run(); err != nil {
			log.Printf("❌ Failed to add UFW rule: %v", err)
			return err
		}

		log.Printf("✅ Port %s/udp allowed in UFW.", port)
	} else {
		log.Println("ℹ️  UFW is installed but inactive. Skipping port configuration.")
	}

	return nil
}

// StartWireGuard brings the wg0 interface UP
func StartWireGuard() error {
	if runtime.GOOS != "linux" {
		return nil
	}

	log.Println("🚀 Bringing WireGuard interface (wg0) UP...")
	cmd := exec.Command("wg-quick", "up", "wg0")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start wireguard (is it already running?): %v", err)
	}
	return nil
}

// StopWireGuard brings the wg0 interface DOWN
func StopWireGuard() error {
	if runtime.GOOS != "linux" {
		return nil
	}

	log.Println("🛑 Bringing WireGuard interface (wg0) DOWN...")
	cmd := exec.Command("wg-quick", "down", "wg0")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to stop wireguard: %v", err)
	}
	return nil
}

// EnableIPForwarding tells the Linux kernel to allow routing packets between interfaces.
func EnableIPForwarding() error {
	if runtime.GOOS != "linux" {
		return nil
	}
	log.Println("⚙️ Enabling IPv4 Forwarding in the Linux kernel...")

	err := os.WriteFile("/proc/sys/net/ipv4/ip_forward", []byte("1\n"), 0644)
	if err != nil {
		return fmt.Errorf("failed to enable ip_forwarding: %v", err)
	}

	exec.Command("sysctl", "-w", "net.ipv4.ip_forward=1").Run()
	return nil
}

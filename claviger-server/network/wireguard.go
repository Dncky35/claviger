package network

import (
	"encoding/base64"
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

	// Check if 'wg' command already exists
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

	// Run 'apt-get install wireguard'
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

// WriteBasicConfig writes the standard wg0.conf file to the Linux filesystem (Without Admin)
func WriteBasicConfig(privateKey string) error {
	if runtime.GOOS != "linux" {
		return nil // Skip file creation on Windows testing
	}

	configPath := "/etc/wireguard/wg0.conf"

	// Create the /etc/wireguard directory with strict 0700 security permissions
	os.MkdirAll("/etc/wireguard", 0700)

	// A very standard starting template for a WireGuard server
	configContent := fmt.Sprintf(`[Interface]
PrivateKey = %s
Address = 10.8.0.1/24
ListenPort = 51820
SaveConfig = true

# The Claviger daemon will dynamically append [Peer] blocks below!
`, privateKey)

	// Write the file (Requires root/sudo privileges!)
	err := os.WriteFile(configPath, []byte(configContent), 0600)
	if err != nil {
		return fmt.Errorf("failed to write wg0.conf: %v", err)
	}

	log.Println("✅ WireGuard configuration (wg0.conf) created successfully.")
	return nil
}

// GetPublicIP automatically finds the server's public IP address
func GetPublicIP() (string, error) {
	resp, err := http.Get("https://api.ipify.org")
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

// WriteConfigWithAdmin creates the server config AND adds the Admin as the first peer
func WriteConfigWithAdmin(serverPrivKey, adminPubKey string) error {
	if runtime.GOOS != "linux" {
		return nil
	}

	configContent := fmt.Sprintf(`[Interface]
PrivateKey = %s
Address = 10.8.0.1/24
ListenPort = 51820
SaveConfig = true

# [Peer 1: Super Admin]
[Peer]
PublicKey = %s
AllowedIPs = 10.8.0.2/32
`, serverPrivKey, adminPubKey)

	// Strict directory permissions
	os.MkdirAll("/etc/wireguard", 0700)
	return os.WriteFile("/etc/wireguard/wg0.conf", []byte(configContent), 0600)
}

func GetPeerCounts() (total int, active int) {
	if runtime.GOOS != "linux" {
		return 0, 0
	}

	client, err := wgctrl.New()
	if err != nil {
		log.Printf("⚠️ Failed to connect to WireGuard interface: %v", err)
		return 0, 0
	}
	defer client.Close()

	device, err := client.Device("wg0")
	if err != nil {
		// Normal if the interface is temporarily down
		return 0, 0
	}

	totalCount := len(device.Peers)
	activeCount := 0

	for _, peer := range device.Peers {
		// A peer is considered "online" if it has communicated in the last 3 minutes
		if !peer.LastHandshakeTime.IsZero() && time.Since(peer.LastHandshakeTime) < 3*time.Minute {
			activeCount++
		}
	}

	return totalCount, activeCount
}

// GenerateAdminToken creates the client config and wraps it in a Base64 token
func GenerateAdminToken(adminPrivKey, serverPubKey, serverIP string) string {
	clientConf := fmt.Sprintf(`[Interface]
PrivateKey = %s
Address = 10.8.0.2/24
DNS = 1.1.1.1

[Peer]
PublicKey = %s
Endpoint = %s:51820
AllowedIPs = 0.0.0.0/0
PersistentKeepalive = 25
`, adminPrivKey, serverPubKey, serverIP)

	// Encode it to a single, copy-pasteable string
	encoded := base64.StdEncoding.EncodeToString([]byte(clientConf))
	return "claviger://" + encoded
}

// CheckAndOpenFirewall ensures the WireGuard port is open if UFW is active.
func CheckAndOpenFirewall(port string) error {
	if runtime.GOOS != "linux" {
		return nil
	}

	// 1. Check if ufw is installed and active
	cmd := exec.Command("ufw", "status")
	output, err := cmd.Output()
	if err != nil {
		log.Println("⚠️ UFW not found or requires root. Skipping local firewall rules.")
		return nil
	}

	// 2. Parse the output to see if it's active
	if strings.Contains(string(output), "Status: active") {
		log.Printf("🛡️ UFW is active. Opening port %s/udp...", port)

		allowCmd := exec.Command("ufw", "allow", fmt.Sprintf("%s/udp", port))
		if err := allowCmd.Run(); err != nil {
			return fmt.Errorf("failed to open firewall port: %v", err)
		}
		log.Println("✅ Firewall port opened successfully.")
	} else {
		log.Println("ℹ️ UFW is inactive. No local firewall changes needed.")
	}

	return nil
}

// StartWireGuard brings the wg0 interface UP
func StartWireGuard() error {
	if runtime.GOOS != "linux" {
		log.Println("⚠️ Skipping wg-quick up: Not running on Linux.")
		return nil
	}

	log.Println("🚀 Bringing WireGuard interface (wg0) UP...")

	// We use wg-quick which handles the heavy lifting of ip routes and iptables
	cmd := exec.Command("wg-quick", "up", "wg0")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start wireguard (is it already running?): %v", err)
	}

	log.Println("✅ WireGuard interface is live.")
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

	log.Println("✅ WireGuard interface is offline.")
	return nil
}

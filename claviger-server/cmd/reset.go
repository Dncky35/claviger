package cmd

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"claviger-server/internal/system"
	"claviger-server/network"
	"claviger-server/storage"
)

// ensureSSHAccess acts as a safety net so admins don't get locked out
func ensureSSHAccess() {
	if runtime.GOOS != "linux" {
		return
	}
	fmt.Println("🛡️  Ensuring SSH (Port 22) is open to prevent lockout...")

	cmd := exec.Command("ufw", "status")
	out, err := cmd.Output()
	if err == nil && strings.Contains(string(out), "Status: active") {
		exec.Command("ufw", "allow", "22/tcp").Run()
		fmt.Println("✅ UFW rule added for Port 22 (SSH).")
		return
	}

	exec.Command("iptables", "-I", "INPUT", "-p", "tcp", "--dport", "22", "-j", "ACCEPT").Run()
	fmt.Println("✅ iptables fallback rule added for Port 22 (SSH).")
}

func RunReset() {
	if os.Geteuid() != 0 {
		log.Fatal("❌ Permission Denied. Reset must be run with root privileges (e.g., 'sudo claviger-server reset')")
	}

	fmt.Println("⚠️  Resetting Claviger Edge Node...")

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Are you sure you want to reset this node? This will stop the VPN and wipe all configuration. [y/N]: ")
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))

	if input != "y" && input != "yes" {
		fmt.Println("❌ Reset cancelled. Local configuration was not changed.")
		return
	}

	// 1. Deploy Safety Net
	ensureSSHAccess()

	// --- NEW: Remove Background Service ---
	// We must do this before shutting down the network, otherwise
	// systemd will instantly try to restart the daemon!
	if err := system.RemoveSystemdService(); err != nil {
		log.Printf("⚠️ Warning: %v\n", err)
	}

	// 2. Gracefully stop the VPN
	network.StopWireGuard()

	// 3. Delete the physical WireGuard configuration file
	fmt.Println("🗑️  Removing WireGuard configuration files...")
	if err := os.Remove("/etc/wireguard/wg0.conf"); err != nil && !os.IsNotExist(err) {
		fmt.Printf("⚠️  Could not remove wg0.conf: %v\n", err)
	}

	// 4. Wipe the SQLite database state
	db := storage.InitDB()
	defer db.Close()
	storage.ClearConfig(db)

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("✅ NODE FACTORY RESET COMPLETE")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("You can now run 'sudo claviger-server setup' to attach this server to a new Cloudrocean license.")
}

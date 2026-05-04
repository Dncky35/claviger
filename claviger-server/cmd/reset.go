package cmd

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"claviger-server/internal/apps" // Required for the cascade teardown
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

// =====================================================================
// COMMAND: RESET (Factory Reset the Configuration)
// =====================================================================
func RunReset() {
	if os.Geteuid() != 0 {
		log.Fatal("❌ Permission Denied. Reset must be run with root privileges (e.g., 'sudo claviger-server reset')")
	}

	fmt.Println("⚠️  Resetting Claviger Edge Node...")

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Are you sure you want to reset this node? This will wipe all apps, stop the VPN, and clear all configurations. [y/N]: ")
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))

	if input != "y" && input != "yes" {
		fmt.Println("❌ Reset cancelled. Local configuration was not changed.")
		return
	}

	// 1. The Teardown Cascade (Surgical Strike on Docker Apps)
	fmt.Println("🗑️  Destroying installed extensions and wiping container data...")
	for appID := range apps.Catalog {
		// We attempt to uninstall every app in the registry.
		// If it's not installed, the error is ignored.
		if err := apps.Uninstall(appID); err == nil {
			fmt.Printf("   ✅ Removed %s\n", appID)
		}
	}

	// 2. Deploy Safety Net
	ensureSSHAccess()

	// 3. Remove Background Service
	if err := system.RemoveSystemdService(); err != nil {
		log.Printf("⚠️ Warning: %v\n", err)
	}

	// 4. Gracefully stop the VPN
	fmt.Println("🔌 Stopping WireGuard network...")
	network.StopWireGuard()

	// 5. Delete the physical WireGuard configuration file
	fmt.Println("🗑️  Removing WireGuard configuration files...")
	if err := os.Remove("/etc/wireguard/wg0.conf"); err != nil && !os.IsNotExist(err) {
		fmt.Printf("⚠️  Could not remove wg0.conf: %v\n", err)
	}

	// 6. Wipe the SQLite database state
	fmt.Println("💾 Clearing database configurations...")
	db := storage.InitDB()
	defer db.Close()
	storage.ClearConfig(db)

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("✅ NODE FACTORY RESET COMPLETE")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("You can now run 'sudo claviger-server setup' to re-provision this server.")
}

// =====================================================================
// COMMAND: UNINSTALL (The Scorched Earth Nuclear Option)
// =====================================================================
func RunUninstall() {
	if os.Geteuid() != 0 {
		log.Fatal("❌ Permission Denied. Uninstall must be run with root privileges (e.g., 'sudo claviger-server uninstall')")
	}

	fmt.Println("🧨 UNINSTALLING CLAVIGER EDGE NODE")

	reader := bufio.NewReader(os.Stdin)
	fmt.Print("WARNING: Are you absolutely sure? This will delete the database, all apps, and the Claviger binary itself. [y/N]: ")
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))

	if input != "y" && input != "yes" {
		fmt.Println("❌ Uninstall cancelled.")
		return
	}

	// 1. The Teardown Cascade
	fmt.Println("🗑️  Destroying installed extensions...")
	for appID := range apps.Catalog {
		_ = apps.Uninstall(appID)
	}

	// 2. Deploy Safety Net
	ensureSSHAccess()

	// 3. Remove Background Service
	_ = system.RemoveSystemdService()

	// 4. Gracefully stop the VPN
	fmt.Println("🔌 Stopping WireGuard network...")
	network.StopWireGuard()

	// 5. Scorched Earth Data Wipe
	fmt.Println("🔥 Wiping all system files, databases, and backups...")
	os.Remove("/etc/wireguard/wg0.conf")

	// This physically deletes the SQLite db, the backup keys, the app folders... everything.
	if err := os.RemoveAll("/var/lib/claviger"); err != nil {
		fmt.Printf("⚠️  Could not completely wipe /var/lib/claviger: %v\n", err)
	}

	// 6. Self-Delete the Executable
	fmt.Println("💣 Removing Claviger binary...")
	binaryPath, err := os.Executable()
	if err == nil {
		os.Remove(binaryPath)
	}

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("✅ CLAVIGER COMPLETELY UNINSTALLED")
	fmt.Println(strings.Repeat("=", 60))
}

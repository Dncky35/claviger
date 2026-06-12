package system

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/user"
	"runtime"
)

// InstallSystemdService creates a persistent background service for Claviger
func InstallSystemdService() error {
	if runtime.GOOS != "linux" {
		log.Println("⚠️ Skipping systemd installation: Operating system is not Linux.")
		return nil
	}

	log.Println("⚙️  Configuring systemd auto-start service...")

	// 1. Get the absolute path to the executable
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not find executable path: %v", err)
	}

	// 2. Define the strict Linux data directory
	workDir := "/etc/claviger"
	os.MkdirAll(workDir, 0755)

	// 3. Resolve the actual human user behind the sudo command
	realUser := os.Getenv("SUDO_USER")
	if realUser == "" || realUser == "root" {
		u, err := user.Current()
		if err == nil {
			realUser = u.Username
		} else {
			realUser = "root"
		}
	}
	log.Printf("👤 Detected management user for SSH operations: %s", realUser)

	// 4. Define the systemd service file content
	serviceContent := fmt.Sprintf(`[Unit]
Description=Claviger Edge VPN Daemon
After=network.target network-online.target

[Service]
Type=simple
User=root
WorkingDirectory=%s
ExecStart=%s start
Restart=on-failure
RestartSec=5
LimitNOFILE=65536

# Passes the real human user down to the background daemon context for SSH management
Environment="CLAVIGER_SSH_USER=%s"

[Install]
WantedBy=multi-user.target
`, workDir, execPath, realUser)

	servicePath := "/etc/systemd/system/claviger-server.service"

	// 5. Write the file to the systemd directory
	err = os.WriteFile(servicePath, []byte(serviceContent), 0644)
	if err != nil {
		return fmt.Errorf("failed to write systemd service file: %v", err)
	}

	// 6. Reload systemd so it sees the new file
	if err := exec.Command("systemctl", "daemon-reload").Run(); err != nil {
		return fmt.Errorf("failed to reload systemctl: %v", err)
	}

	// 7. Enable the service so it starts on boot
	if err := exec.Command("systemctl", "enable", "claviger-server.service").Run(); err != nil {
		return fmt.Errorf("failed to enable claviger-server service: %v", err)
	}

	log.Println("✅ Auto-start service (claviger-server) configured successfully.")
	return nil
}

// RemoveSystemdService stops, disables, and deletes the Claviger background service
func RemoveSystemdService() error {
	if runtime.GOOS != "linux" {
		return nil
	}

	servicePath := "/etc/systemd/system/claviger-server.service"

	// If the file doesn't exist, there is nothing to clean up
	if _, err := os.Stat(servicePath); os.IsNotExist(err) {
		return nil
	}

	fmt.Println("🛑 Stopping and removing systemd background service...")

	// 1. Stop the currently running service
	exec.Command("systemctl", "stop", "claviger-server.service").Run()

	// 2. Disable it from starting on boot
	exec.Command("systemctl", "disable", "claviger-server.service").Run()

	// 3. Delete the actual service file
	if err := os.Remove(servicePath); err != nil {
		return fmt.Errorf("failed to delete service file: %v", err)
	}

	// 4. Reload systemd so it forgets the service existed
	exec.Command("systemctl", "daemon-reload").Run()

	return nil
}

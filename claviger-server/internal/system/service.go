package system

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// InstallSystemdService creates a persistent background service for Claviger
func InstallSystemdService() error {
	if runtime.GOOS != "linux" {
		log.Println("⚠️ Skipping systemd installation: Operating system is not Linux.")
		return nil
	}

	log.Println("⚙️  Configuring systemd auto-start service...")

	// 1. Get the absolute path to the currently running Claviger executable
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not find executable path: %v", err)
	}

	// 2. Get the working directory (where claviger.db is stored)
	workDir := filepath.Dir(execPath)

	// 3. Define the systemd service file content
	serviceContent := fmt.Sprintf(`[Unit]
Description=Claviger Edge VPN Daemon
After=network.target network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
WorkingDirectory=%s
ExecStart=%s start
Restart=on-failure
RestartSec=5
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
`, workDir, execPath)

	servicePath := "/etc/systemd/system/claviger.service"

	// 4. Write the file to the systemd directory
	err = os.WriteFile(servicePath, []byte(serviceContent), 0644)
	if err != nil {
		return fmt.Errorf("failed to write systemd service file: %v", err)
	}

	// 5. Reload systemd so it sees the new file
	if err := exec.Command("systemctl", "daemon-reload").Run(); err != nil {
		return fmt.Errorf("failed to reload systemctl: %v", err)
	}

	// 6. Enable the service so it starts on boot
	if err := exec.Command("systemctl", "enable", "claviger.service").Run(); err != nil {
		return fmt.Errorf("failed to enable claviger service: %v", err)
	}

	log.Println("✅ Auto-start service configured successfully.")
	return nil
}

// RemoveSystemdService stops, disables, and deletes the Claviger background service
func RemoveSystemdService() error {
	if runtime.GOOS != "linux" {
		return nil
	}

	servicePath := "/etc/systemd/system/claviger.service"

	// If the file doesn't exist, there is nothing to clean up
	if _, err := os.Stat(servicePath); os.IsNotExist(err) {
		return nil
	}

	fmt.Println("🛑 Stopping and removing systemd background service...")

	// 1. Stop the currently running service
	exec.Command("systemctl", "stop", "claviger.service").Run()

	// 2. Disable it from starting on boot
	exec.Command("systemctl", "disable", "claviger.service").Run()

	// 3. Delete the actual service file
	if err := os.Remove(servicePath); err != nil {
		return fmt.Errorf("failed to delete service file: %v", err)
	}

	// 4. Reload systemd so it forgets the service existed
	exec.Command("systemctl", "daemon-reload").Run()

	return nil
}

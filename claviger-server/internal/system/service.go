package system

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/user"
	"runtime"
)

// InstallSystemdService creates persistent background services for Claviger (Server + Updater)
func InstallSystemdService() error {
	if runtime.GOOS != "linux" {
		log.Println("⚠️ Skipping systemd installation: Operating system is not Linux.")
		return nil
	}

	log.Println("⚙️ Configuring systemd auto-start services...")

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

	// --- 4. Write Main Server Service ---
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
	if err = os.WriteFile(servicePath, []byte(serviceContent), 0644); err != nil {
		return fmt.Errorf("failed to write systemd service file: %v", err)
	}

	// --- 5. Write Persistent Isolated Updater Service ---
	// This runs completely outside the server's cgroup, preventing the "murder-suicide" trap during updates.
	updaterServiceContent := `[Unit]
Description=Claviger Independent System Updater
After=network.target

[Service]
Type=oneshot
ExecStart=/bin/bash -c 'if [ -f /etc/claviger/pending_update.url ]; then URL=$(cat /etc/claviger/pending_update.url); rm -f /etc/claviger/pending_update.url; curl -sSL "$URL" | bash; fi'
`
	updaterPath := "/etc/systemd/system/claviger-updater.service"
	if err = os.WriteFile(updaterPath, []byte(updaterServiceContent), 0644); err != nil {
		return fmt.Errorf("failed to write updater service file: %v", err)
	}

	// 6. Reload systemd so it sees both files
	if err := exec.Command("systemctl", "daemon-reload").Run(); err != nil {
		return fmt.Errorf("failed to reload systemctl: %v", err)
	}

	// 7. Enable the main service so it starts on boot
	if err := exec.Command("systemctl", "enable", "claviger-server.service").Run(); err != nil {
		return fmt.Errorf("failed to enable claviger-server service: %v", err)
	}

	log.Println("✅ Auto-start services (claviger-server & claviger-updater) configured successfully.")
	return nil
}

// RemoveSystemdService stops, disables, and deletes the Claviger background services
func RemoveSystemdService() error {
	if runtime.GOOS != "linux" {
		return nil
	}

	services := []string{
		"/etc/systemd/system/claviger-server.service",
		"/etc/systemd/system/claviger-updater.service",
	}

	fmt.Println("🛑 Stopping and removing systemd background services...")

	for _, svcPath := range services {
		if _, err := os.Stat(svcPath); os.IsNotExist(err) {
			continue
		}
		// Extract service name for systemctl calls
		svcName := "claviger-server.service"
		if svcPath == "/etc/systemd/system/claviger-updater.service" {
			svcName = "claviger-updater.service"
		}

		exec.Command("systemctl", "stop", svcName).Run()
		exec.Command("systemctl", "disable", svcName).Run()
		os.Remove(svcPath)
	}

	exec.Command("systemctl", "daemon-reload").Run()
	return nil
}

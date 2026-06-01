package security

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type Fail2BanStats struct {
	CurrentlyFailed int      `json:"currently_failed"`
	TotalFailed     int      `json:"total_failed"`
	CurrentlyBanned int      `json:"currently_banned"`
	TotalBanned     int      `json:"total_banned"`
	BannedIPs       []string `json:"banned_ips"`
}

// IsFail2BanInstalled checks if the binary exists on the system
func IsFail2BanInstalled() bool {
	_, err := exec.LookPath("fail2ban-client")
	return err == nil
}

// UninstallFail2Ban completely removes the engine and purges all configuration files
func UninstallFail2Ban() error {
	log.Println("🧹 Starting Fail2Ban uninstallation process...")

	// 1. Stop and disable the service gracefully (ignore errors if already stopped)
	exec.Command("systemctl", "stop", "fail2ban").Run()
	exec.Command("systemctl", "disable", "fail2ban").Run()

	// 2. Purge the package using apt-get
	// 'purge' tells APT to delete the package AND its default config files
	purgeCmd := exec.Command("apt-get", "purge", "-y", "fail2ban")
	purgeCmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
	if err := purgeCmd.Run(); err != nil {
		return fmt.Errorf("failed to purge fail2ban package: %v", err)
	}

	// Clean up any orphaned dependencies Fail2Ban might have brought in
	exec.Command("apt-get", "autoremove", "-y").Run()

	// 3. Force wipe the entire configuration directory
	// APT sometimes leaves behind user-modified files (like our custom jail.d configs)
	if err := os.RemoveAll("/etc/fail2ban"); err != nil {
		return fmt.Errorf("failed to delete /etc/fail2ban directory: %v", err)
	}

	// 4. Reload the systemd daemon to clear the missing service
	exec.Command("systemctl", "daemon-reload").Run()

	return nil
}

// IsFail2BanRunning checks systemd for the active state
func IsFail2BanRunning() bool {
	err := exec.Command("systemctl", "is-active", "--quiet", "fail2ban").Run()
	return err == nil
}

// InstallFail2Ban safely installs via apt if it doesn't exist
func InstallFail2Ban() error {
	if IsFail2BanInstalled() {
		// It's already here! Just make sure it's running.
		exec.Command("systemctl", "enable", "--now", "fail2ban").Run()
		return nil
	}

	// Update and Install silently
	if err := exec.Command("apt-get", "update").Run(); err != nil {
		return fmt.Errorf("failed to update apt: %v", err)
	}

	if err := exec.Command("apt-get", "install", "-y", "fail2ban").Run(); err != nil {
		return fmt.Errorf("failed to install fail2ban: %v", err)
	}

	// Enable and Start
	if err := exec.Command("systemctl", "enable", "--now", "fail2ban").Run(); err != nil {
		return fmt.Errorf("failed to enable fail2ban service: %v", err)
	}

	return nil
}

// ConfigureFail2Ban safely writes to jail.d/ without breaking user configs
func ConfigureFail2Ban(port int, maxRetry int, findTimeMins int, banTimeMins int) error {
	if !IsFail2BanInstalled() {
		return fmt.Errorf("fail2ban is not installed")
	}

	// Convert minutes to seconds for Fail2Ban format
	findTimeSecs := findTimeMins * 60
	banTimeSecs := banTimeMins * 60
	if banTimeMins == -1 {
		banTimeSecs = -1 // -1 means permanent ban
	}

	config := fmt.Sprintf(`[sshd]
enabled = true
port = %d
filter = sshd
logpath = /var/log/auth.log
maxretry = %d
findtime = %d
bantime = %d
`, port, maxRetry, findTimeSecs, banTimeSecs)

	jailDir := "/etc/fail2ban/jail.d"
	os.MkdirAll(jailDir, 0755)

	filePath := filepath.Join(jailDir, "claviger-sshd.conf")
	if err := os.WriteFile(filePath, []byte(config), 0644); err != nil {
		return fmt.Errorf("failed to write fail2ban config: %v", err)
	}

	if err := exec.Command("systemctl", "restart", "fail2ban").Run(); err != nil {
		return fmt.Errorf("failed to restart fail2ban: %v", err)
	}

	return nil
}

// GetFail2BanStats parses the command line output of fail2ban-client
func GetFail2BanStats() Fail2BanStats {
	stats := Fail2BanStats{
		BannedIPs: []string{}, // Initialize empty so JSON doesn't return null
	}

	// Note: "sshd" is the name of the jail we created in your setup script
	out, err := exec.Command("fail2ban-client", "status", "sshd").Output()
	if err != nil {
		return stats // Return empty stats if it fails
	}

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if strings.Contains(line, "Currently failed:") {
			parts := strings.Split(line, ":")
			if len(parts) == 2 {
				stats.CurrentlyFailed, _ = strconv.Atoi(strings.TrimSpace(parts[1]))
			}
		} else if strings.Contains(line, "Total failed:") {
			parts := strings.Split(line, ":")
			if len(parts) == 2 {
				stats.TotalFailed, _ = strconv.Atoi(strings.TrimSpace(parts[1]))
			}
		} else if strings.Contains(line, "Currently banned:") {
			parts := strings.Split(line, ":")
			if len(parts) == 2 {
				stats.CurrentlyBanned, _ = strconv.Atoi(strings.TrimSpace(parts[1]))
			}
		} else if strings.Contains(line, "Total banned:") {
			parts := strings.Split(line, ":")
			if len(parts) == 2 {
				stats.TotalBanned, _ = strconv.Atoi(strings.TrimSpace(parts[1]))
			}
		} else if strings.Contains(line, "Banned IP list:") {
			parts := strings.Split(line, ":")
			if len(parts) == 2 {
				ipString := strings.TrimSpace(parts[1])
				if ipString != "" {
					// strings.Fields perfectly splits by spaces, tabs, etc.
					stats.BannedIPs = strings.Fields(ipString)
				}
			}
		}
	}

	return stats
}

// UnbanIP safely removes an IP from the jail
func UnbanIP(ip string) error {
	// Security Failsafe: Ensure it's a real IP address before passing to terminal
	if net.ParseIP(ip) == nil {
		return fmt.Errorf("invalid IP format")
	}
	return exec.Command("fail2ban-client", "set", "sshd", "unbanip", ip).Run()
}

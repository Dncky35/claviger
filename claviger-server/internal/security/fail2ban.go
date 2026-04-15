package security

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// IsFail2BanInstalled checks if the binary exists on the system
func IsFail2BanInstalled() bool {
	_, err := exec.LookPath("fail2ban-client")
	return err == nil
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

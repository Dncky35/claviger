package vpn

import (
	"claviger-client/internal/config"
	"fmt"
	"log"
	"os/exec"
	"runtime"
)

// Engine manages the local WireGuard network interface
type Engine struct {
	isRunning bool
}

// NewEngine creates a new VPN manager
func NewEngine() *Engine {
	return &Engine{
		isRunning: false,
	}
}

// Connect builds the config file and asks the OS to establish the tunnel
func (e *Engine) Connect(vault *config.ClientVault) error {
	if e.isRunning {
		return fmt.Errorf("VPN is already connected")
	}

	log.Printf("🚀 Starting Claviger VPN Engine...")
	log.Printf("📍 Assigned IP: %s", vault.AssignedIP)
	log.Printf("🎯 Target Endpoint: %s", vault.ServerEndpoint)

	// 1. Generate the claviger.conf file securely on the hard drive
	configPath, err := WriteConfigFile(vault)
	if err != nil {
		return fmt.Errorf("failed to generate config: %v", err)
	}

	// 2. Execute the elevated OS command (wg-quick up /tmp/claviger.conf)
	log.Printf("🔑 Requesting OS Administrator privileges to build network tunnel...")
	err = executeElevated("wg-quick", "up", configPath)
	if err != nil {
		return fmt.Errorf("failed to start VPN tunnel: %v", err)
	}

	e.isRunning = true
	log.Println("✅ Secure tunnel established!")
	return nil
}

// Disconnect destroys the virtual network card and restores normal internet
func (e *Engine) Disconnect() error {
	if !e.isRunning {
		return nil
	}

	log.Println("🛑 Shutting down VPN tunnel...")

	// Find where we saved the config file
	configPath := GetConfigPath()

	// Tell the OS to tear down the tunnel
	err := executeElevated("wg-quick", "down", configPath)
	if err != nil {
		log.Printf("⚠️ Warning during disconnect (tunnel might already be down): %v", err)
	}

	e.isRunning = false
	log.Println("✅ Disconnected.")
	return nil
}

// Status returns whether the tunnel is currently active
func (e *Engine) Status() bool {
	return e.isRunning
}

// ==========================================
// OS-SPECIFIC PRIVILEGE ESCALATION
// ==========================================

// executeElevated handles cross-platform admin prompts to run system-level networking commands
func executeElevated(command, action, target string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "linux":
		// 'pkexec' triggers the native graphical Linux password prompt (PolicyKit)
		cmd = exec.Command("pkexec", command, action, target)

	case "windows":
		// Windows doesn't use 'wg-quick'. It uses 'wireguard.exe' to install a background service.
		if command == "wg-quick" {
			switch action {
			case "up":
				// We wrap the target path in quotes in case there are spaces in the Windows username
				psCommand := fmt.Sprintf("Start-Process 'C:\\Program Files\\WireGuard\\wireguard.exe' -ArgumentList '/installtunnelservice','%s' -Verb RunAs -Wait", target)
				cmd = exec.Command("powershell", "-Command", psCommand)
			case "down":
				// To uninstall, WireGuard just needs the name of the file without the .conf extension
				psCommand := "Start-Process 'C:\\Program Files\\WireGuard\\wireguard.exe' -ArgumentList '/uninstalltunnelservice','claviger' -Verb RunAs -Wait"
				cmd = exec.Command("powershell", "-Command", psCommand)
			}
		} else {
			return fmt.Errorf("unsupported Windows command: %s", command)
		}

	case "darwin":
		// macOS native AppleScript prompt
		macCmd := fmt.Sprintf("do shell script \"%s %s %s\" with administrator privileges", command, action, target)
		cmd = exec.Command("osascript", "-e", macCmd)

	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}

	// Run the command and capture any terminal errors
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("OS execution failed: %v | output: %s", err, string(output))
	}

	return nil
}

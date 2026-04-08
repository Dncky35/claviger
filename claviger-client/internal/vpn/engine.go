package vpn

import (
	"claviger-client/internal/config"
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// The 5 Exact States of our Zero-Trust Tunnel
const (
	StateDisconnected = "disconnected" // ⚪ Off
	StateConnecting   = "connecting"   // 🟡 Building tunnel
	StateVerifying    = "verifying"    // 🔵 Waiting for first handshake
	StateSecured      = "secured"      // 🟢 Traffic is flowing securely
	StateReconnecting = "reconnecting" // 🟠 Ghost connection (Server unresponsive)
)

// Engine manages the embedded WireGuard network interface
type Engine struct {
	wgDevice *device.Device

	// State Management
	currentState  string
	stateMutex    sync.RWMutex // Protects currentState from thread crashes
	stateCallback func(string) // Wails Event Hook!

	// Watchdog Management
	watchdogCtx    context.Context
	watchdogCancel context.CancelFunc
}

// NewEngine creates a new VPN manager
func NewEngine() *Engine {
	return &Engine{
		currentState: StateDisconnected,
	}
}

// ==========================================
// STATE MANAGEMENT THREAD SAFETY
// ==========================================

// SetStateCallback allows the main Wails app to listen for background state changes
func (e *Engine) SetStateCallback(cb func(string)) {
	e.stateMutex.Lock()
	defer e.stateMutex.Unlock()
	e.stateCallback = cb
}

// setState safely updates the engine's status and triggers the Wails Event
func (e *Engine) setState(newState string) {
	e.stateMutex.Lock()

	changed := false
	if e.currentState != newState {
		log.Printf("🔄 Tunnel State Changed: %s -> %s", e.currentState, newState)
		e.currentState = newState
		changed = true
	}

	// Copy the callback reference while locked
	cb := e.stateCallback
	e.stateMutex.Unlock() // Release lock BEFORE calling the callback to prevent deadlocks

	// Fire the Wails Event!
	if changed && cb != nil {
		cb(newState)
	}
}

// GetState safely reads the current status for the UI
func (e *Engine) GetState() string {
	e.stateMutex.RLock() // RLock allows multiple reads at once, but blocks writes
	defer e.stateMutex.RUnlock()
	return e.currentState
}

// ==========================================
// GHOST CONNECTION DETECTION (THE WATCHDOG)
// ==========================================

// getLastHandshake queries the embedded WireGuard device for the exact Unix timestamp of the last key exchange
func (e *Engine) getLastHandshake() (time.Time, error) {
	if e.wgDevice == nil {
		return time.Time{}, fmt.Errorf("device is nil")
	}

	// IpcGet() dumps the raw internal status of the device
	stats, err := e.wgDevice.IpcGet()
	if err != nil {
		return time.Time{}, err
	}

	lines := strings.Split(stats, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "last_handshake_time_sec=") {
			secStr := strings.TrimPrefix(line, "last_handshake_time_sec=")
			sec, err := strconv.ParseInt(secStr, 10, 64)
			if err != nil {
				return time.Time{}, err
			}
			return time.Unix(sec, 0), nil
		}
	}
	return time.Time{}, fmt.Errorf("no handshake data found")
}

// startWatchdog runs continuously in the background while the VPN is active
func (e *Engine) startWatchdog() {
	e.watchdogCtx, e.watchdogCancel = context.WithCancel(context.Background())

	go func(ctx context.Context) {
		ticker := time.NewTicker(5 * time.Second) // Check every 5 seconds
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				// User clicked Disconnect. Kill the watchdog gracefully.
				log.Println("🛑 Watchdog loop terminated.")
				return

			case <-ticker.C:
				lastHandshake, err := e.getLastHandshake()
				if err != nil {
					continue
				}

				timeSince := time.Since(lastHandshake)
				currentState := e.GetState()

				// STATE MACHINE LOGIC
				switch currentState {
				case StateVerifying:
					// 1. Did we get a handshake right away? (Unix() > 0 ensures it's not the year 1970)
					if lastHandshake.Unix() > 0 && timeSince < 2*time.Minute {
						e.setState(StateSecured)
					} else if timeSince > 15*time.Second {
						// 2. We've been trying for 15 seconds and no handshake. Server is unresponsive!
						e.setState(StateReconnecting)
					}
				case StateSecured:
					// 3. We were secured, but the handshake is getting old. We lost connection!
					if timeSince > 3*time.Minute {
						e.setState(StateReconnecting)
					}
				case StateReconnecting:
					// 4. The connection healed itself! Handshake resumed.
					if lastHandshake.Unix() > 0 && timeSince < 2*time.Minute {
						e.setState(StateSecured)
					}
				}
			}
		}
	}(e.watchdogCtx)
}

// ==========================================
// OS-SPECIFIC TUNNEL ROUTING
// ==========================================

// assignOSTunnelIP tells the operating system to send 10.8.0.x traffic into our memory tunnel
func assignOSTunnelIP(interfaceName, assignedIP string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "linux":
		// Linux uses 'ip' commands to bring up the link and assign the IP
		exec.Command("ip", "link", "set", "dev", interfaceName, "up").Run()
		cmd = exec.Command("ip", "address", "add", assignedIP+"/24", "dev", interfaceName)

	case "windows":
		// Windows uses netsh to assign the IP
		cmd = exec.Command("netsh", "interface", "ipv4", "set", "address",
			fmt.Sprintf("name=\"%s\"", interfaceName),
			"static", assignedIP, "255.255.255.0", "none",
		)

	case "darwin":
		// macOS uses ifconfig
		cmd = exec.Command("ifconfig", interfaceName, assignedIP, assignedIP, "up")
		exec.Command("route", "-n", "add", "-net", "10.8.0.0/24", "-interface", interfaceName).Run()

	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("OS command failed: %v | output: %s", err, string(output))
	}
	return nil
}

// ==========================================
// TUNNEL LIFECYCLE CONTROLS
// ==========================================

// Connect creates a virtual interface and configures WireGuard entirely in memory
func (e *Engine) Connect(vault *config.ClientVault) error {
	if e.GetState() != StateDisconnected {
		return fmt.Errorf("VPN is already connected or attempting to connect")
	}

	e.setState(StateConnecting) // 🟡 Update UI

	log.Printf("🚀 Starting Embedded Claviger Engine...")

	interfaceName := "claviger0"
	if runtime.GOOS == "darwin" {
		interfaceName = "utun"
	}

	tunDevice, err := tun.CreateTUN(interfaceName, device.DefaultMTU)
	if err != nil {
		e.setState(StateDisconnected) // Revert on fail
		return fmt.Errorf("failed to create TUN device: %v", err)
	}

	logger := device.NewLogger(device.LogLevelVerbose, "claviger: ")
	e.wgDevice = device.NewDevice(tunDevice, conn.NewDefaultBind(), logger)

	privKey, _ := wgtypes.ParseKey(vault.PrivateKey)
	pubKey, _ := wgtypes.ParseKey(vault.ServerKey)

	uapiConfig := fmt.Sprintf(`private_key=%s
listen_port=0
replace_peers=true
public_key=%s
endpoint=%s
allowed_ip=10.8.0.0/24
persistent_keepalive_interval=25
`,
		hex.EncodeToString(privKey[:]),
		hex.EncodeToString(pubKey[:]),
		vault.ServerEndpoint,
	)

	if err := e.wgDevice.IpcSet(uapiConfig); err != nil {
		e.wgDevice.Close()
		e.setState(StateDisconnected) // Revert on fail
		return fmt.Errorf("failed to configure memory device: %v", err)
	}
	e.wgDevice.Up()

	realInterfaceName, _ := tunDevice.Name()
	if err := assignOSTunnelIP(realInterfaceName, vault.AssignedIP); err != nil {
		e.wgDevice.Close()
		e.setState(StateDisconnected) // Revert on fail
		return fmt.Errorf("failed to route OS traffic: %v", err)
	}

	// SUCCESS! The tunnel exists. Now we wait for Proof of Life.
	e.setState(StateVerifying) // 🔵 Update UI
	e.startWatchdog()          // 🐕 Unleash the hounds

	return nil
}

// Disconnect gracefully destroys the memory interface and cleans up OS routes
func (e *Engine) Disconnect() error {
	if e.GetState() == StateDisconnected {
		return nil
	}

	log.Println("🛑 Shutting down Embedded VPN tunnel...")

	// 1. Kill the Watchdog goroutine FIRST so it stops updating the state
	if e.watchdogCancel != nil {
		e.watchdogCancel()
	}

	// 2. Destroy the memory interface and clear OS routes
	if e.wgDevice != nil {
		e.wgDevice.Close()
		e.wgDevice = nil
	}

	// 3. Tell the UI we are fully shut down
	e.setState(StateDisconnected) // ⚪ Update UI

	log.Println("✅ Disconnected.")
	return nil
}

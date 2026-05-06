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
	stateMutex    sync.RWMutex
	stateCallback func(string)

	// Watchdog Management
	watchdogCtx    context.Context
	watchdogCancel context.CancelFunc

	// Routing Management
	activeServerIP string // 👈 NEW: Tracks the server IP for clean disconnections
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
					// Handshake success! (Unix() > 0 filters out the year 1970 zero-value)
					if lastHandshake.Unix() > 0 && timeSince < 45*time.Second {
						e.setState(StateSecured)
					} else if timeSince > 15*time.Second {
						// 15 seconds with no first handshake = Server unreachable
						e.setState(StateReconnecting)
					}
				case StateSecured:
					// We missed almost two keep-alives (25s * 2). The server is gone!
					if timeSince > 55*time.Second {
						e.setState(StateReconnecting)
					}
				case StateReconnecting:
					// The connection healed itself! Handshake resumed.
					if lastHandshake.Unix() > 0 && timeSince < 45*time.Second {
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

// assignOSTunnelIP tells the operating system how to route traffic into our memory tunnel
func assignOSTunnelIP(interfaceName, assignedIP string, useGlobalRouting bool, serverEndpoint string) error {
	var cmds []*exec.Cmd
	serverIP := strings.Split(serverEndpoint, ":")[0]

	switch runtime.GOOS {
	case "linux":
		cmds = append(cmds, exec.Command("ip", "link", "set", "dev", interfaceName, "up"))
		cmds = append(cmds, exec.Command("ip", "address", "add", assignedIP+"/24", "dev", interfaceName))
		if useGlobalRouting {
			cmds = append(cmds, exec.Command("ip", "route", "add", "0.0.0.0/1", "dev", interfaceName))
			cmds = append(cmds, exec.Command("ip", "route", "add", "128.0.0.0/1", "dev", interfaceName))
		}

	case "windows":
		cmds = append(cmds, exec.Command("netsh", "interface", "ipv4", "set", "address",
			fmt.Sprintf("name=\"%s\"", interfaceName), "static", assignedIP, "255.255.255.0", "none"))

		if useGlobalRouting {
			cmds = append(cmds, exec.Command("netsh", "interface", "ipv4", "set", "dnsservers",
				fmt.Sprintf("name=\"%s\"", interfaceName), "static", "8.8.8.8", "primary"))

			// 🛡️ THE ROUTING LOOP FIX: Find the local Wi-Fi router IP and whitelist the VPN server!
			gwOut, _ := exec.Command("powershell", "-Command", "(Get-NetRoute -DestinationPrefix '0.0.0.0/0' | Sort-Object RouteMetric | Select-Object -First 1).NextHop").Output()
			gateway := strings.TrimSpace(string(gwOut))

			if gateway != "" {
				log.Printf("🛡️ Whitelisting Server IP %s via local gateway %s", serverIP, gateway)
				cmds = append(cmds, exec.Command("route", "add", serverIP, "mask", "255.255.255.255", gateway))
			}

			cmds = append(cmds, exec.Command("netsh", "interface", "ipv4", "add", "route", "0.0.0.0/1", interfaceName, "metric=1", "store=active"))
			cmds = append(cmds, exec.Command("netsh", "interface", "ipv4", "add", "route", "128.0.0.0/1", interfaceName, "metric=1", "store=active"))
		}

	case "darwin":
		cmds = append(cmds, exec.Command("ifconfig", interfaceName, assignedIP, assignedIP, "up"))
		if useGlobalRouting {
			cmds = append(cmds, exec.Command("route", "-n", "add", "-net", "0.0.0.0/1", "-interface", interfaceName))
			cmds = append(cmds, exec.Command("route", "-n", "add", "-net", "128.0.0.0/1", "-interface", interfaceName))
		} else {
			cmds = append(cmds, exec.Command("route", "-n", "add", "-net", "10.8.0.0/24", "-interface", interfaceName))
		}
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}

	for _, cmd := range cmds {
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("OS command failed: %s | output: %s | error: %v", cmd.String(), string(output), err)
		}
	}
	return nil
}

// cleanupGlobalRoutes acts as a fail-safe sweeper.
func (e *Engine) cleanupGlobalRoutes() {
	var cmds []*exec.Cmd

	switch runtime.GOOS {
	case "linux":
		cmds = append(cmds, exec.Command("ip", "route", "del", "0.0.0.0/1"))
		cmds = append(cmds, exec.Command("ip", "route", "del", "128.0.0.0/1"))
	case "windows":
		// Clean up the Server Whitelist route
		if e.activeServerIP != "" {
			cmds = append(cmds, exec.Command("route", "delete", e.activeServerIP, "mask", "255.255.255.255"))
		}
		cmds = append(cmds, exec.Command("netsh", "interface", "ipv4", "delete", "route", "0.0.0.0/1", "claviger0"))
		cmds = append(cmds, exec.Command("netsh", "interface", "ipv4", "delete", "route", "128.0.0.0/1", "claviger0"))
	case "darwin":
		cmds = append(cmds, exec.Command("route", "-n", "delete", "-net", "0.0.0.0/1"))
		cmds = append(cmds, exec.Command("route", "-n", "delete", "-net", "128.0.0.0/1"))
	}

	for _, cmd := range cmds {
		_ = cmd.Run()
	}
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

	// ==========================================
	// THE ROUTING SWITCH (FIXED FOR UAPI SYNTAX)
	// ==========================================
	var allowedIPsBlock string
	if vault.UseGlobalRouting {
		log.Println("🌐 Global Routing enabled: Preparing to route ALL traffic through the tunnel.")
		// UAPI requires each IP block to be declared on its own separate line!
		allowedIPsBlock = "allowed_ip=0.0.0.0/0\nallowed_ip=::/0"
	} else {
		log.Println("🔒 Split Tunnel enabled: Only accessing the secure Hub subnet.")
		allowedIPsBlock = "allowed_ip=10.8.0.0/24"
	}

	// Notice that %s replaced allowed_ip=%s in the template below
	uapiConfig := fmt.Sprintf(`private_key=%s
listen_port=0
replace_peers=true
public_key=%s
endpoint=%s
%s
persistent_keepalive_interval=25
`,
		hex.EncodeToString(privKey[:]),
		hex.EncodeToString(pubKey[:]),
		vault.ServerEndpoint,
		allowedIPsBlock, // Inject our properly formatted multi-line block here!
	)

	if err := e.wgDevice.IpcSet(uapiConfig); err != nil {
		e.wgDevice.Close()
		e.setState(StateDisconnected) // Revert on fail
		return fmt.Errorf("failed to configure memory device: %v", err)
	}
	e.wgDevice.Up()

	realInterfaceName, _ := tunDevice.Name()
	if err := assignOSTunnelIP(realInterfaceName, vault.AssignedIP, vault.UseGlobalRouting, vault.ServerEndpoint); err != nil {
		e.wgDevice.Close()
		e.setState(StateDisconnected)
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

	// 2. THE SWEEPER: Clean up any lingering Global Routing rules before destroying the interface
	log.Println("🧹 Sweeping OS routing tables...")
	e.cleanupGlobalRoutes()

	// 3. Destroy the memory interface and clear OS routes
	if e.wgDevice != nil {
		e.wgDevice.Close()
		e.wgDevice = nil
	}

	// 4. Tell the UI we are fully shut down
	e.setState(StateDisconnected) // ⚪ Update UI

	log.Println("✅ Disconnected. Network routes restored to normal.")
	return nil
}

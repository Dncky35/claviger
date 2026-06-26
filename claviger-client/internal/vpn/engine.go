package vpn

import (
	"claviger-client/internal/config"
	"claviger-client/internal/controller"
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"net"
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
	StateDisconnected = "Disconnected" // ⚪ Off
	StateConnecting   = "Connecting"   // 🟡 Building tunnel
	StateVerifying    = "Verifying"    // 🔵 Waiting for first handshake
	StateSecured      = "Secured"      // 🟢 Traffic is flowing securely
	StateReconnecting = "Reconnecting" // 🟠 Ghost connection (Server unresponsive)
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

	// Lifecycle Management
	syncStatus string // e.g., "Stable", "Syncing...", "Fallback"
	syncMutex  sync.RWMutex
	syncCancel context.CancelFunc // 🎯 Added to kill the sync loop

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

// SetStateCallback allows the main UI app to listen for background state changes
func (e *Engine) SetStateCallback(cb func(string)) {
	e.stateMutex.Lock()
	defer e.stateMutex.Unlock()
	e.stateCallback = cb
}

// setState safely updates the engine's status and triggers the UI Event
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

// SetSyncStatus safely updates the sync state and forces a UI refresh
func (e *Engine) SetSyncStatus(status string) {
	// 1. Safely update the internal sync status
	e.syncMutex.Lock()
	e.syncStatus = status
	e.syncMutex.Unlock()

	// 2. Safely grab the UI callback and current VPN state
	e.stateMutex.RLock()
	cb := e.stateCallback
	currentState := e.currentState // This will be "Secured"
	e.stateMutex.RUnlock()

	// 3. FORCE THE UI REFRESH
	if cb != nil {
		// By calling the callback directly here, we bypass the VPN state deduplication.
		// This forces the UI to re-run your `case vpn.StateSecured:` block
		// and fetch the fresh Sync status!
		cb(currentState)
	}
}

// GetSyncStatus safely reads the current sync status for the UI
func (e *Engine) GetSyncStatus() string {
	e.syncMutex.RLock()
	defer e.syncMutex.RUnlock()

	// If it's empty (e.g., just booted), default to Stable
	if e.syncStatus == "" {
		return controller.SyncStable
	}
	return e.syncStatus
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
		// Start fast (every 2 seconds) to catch the initial connection instantly
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				log.Println("🛑 Watchdog loop terminated.")
				return

			case <-ticker.C:
				lastHandshake, err := e.getLastHandshake()
				if err != nil {
					continue
				}

				timeSince := time.Since(lastHandshake)
				currentState := e.GetState()

				// 🎯 RELAXED STATE MACHINE LOGIC (Handshake Only)
				switch currentState {
				case StateVerifying:
					// As soon as we get our FIRST handshake, we are Secured!
					if lastHandshake.Unix() > 0 && timeSince < 120*time.Second {
						log.Println("✅ First Handshake received! Tunnel is Secured.")
						e.setState(StateSecured)

						// Slow down the ticker to save CPU power while connected
						ticker.Reset(30 * time.Second)
					} else if timeSince > 15*time.Second && lastHandshake.Unix() == 0 {
						// 15 seconds passed and NO first handshake ever arrived
						e.setState(StateReconnecting)
					}

				case StateSecured:
					// WireGuard automatically handshakes every ~2 minutes.
					// If we go 3 full minutes (180s) without a handshake, the server is genuinely offline.
					if timeSince > 180*time.Second {
						log.Println("⚠️ Watchdog: No handshake for 3 minutes. Server might be down.")
						e.setState(StateReconnecting)

						// Speed the ticker back up to quickly detect when it comes back online
						ticker.Reset(5 * time.Second)
					}

				case StateReconnecting:
					// If the handshake magically updates, we are back online!
					if lastHandshake.Unix() > 0 && timeSince < 60*time.Second {
						log.Println("✅ Watchdog: Tunnel recovered!")
						e.setState(StateSecured)
						ticker.Reset(30 * time.Second)
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
func assignOSTunnelIP(interfaceName, assignedIP, dnsSetting, baseSubnet string, useGlobalRouting bool, serverEndpoint string) error {
	// 🎯 Failsafe for older vaults
	if baseSubnet == "" {
		baseSubnet = "10.8.0.0/24"
		log.Println("⚠️ Warning: BaseSubnet was empty, falling back to 10.8.0.0/24")
	}

	var cmds []*exec.Cmd
	serverIP := strings.Split(serverEndpoint, ":")[0]

	// The server might send multiple DNS IPs (e.g., "1.1.1.1, 1.0.0.1").
	// For OS commands, we usually just need the primary one.
	primaryDNS := strings.TrimSpace(strings.Split(dnsSetting, ",")[0])
	if primaryDNS == "" {
		primaryDNS = "1.1.1.1" // Ultimate failsafe
	}

	// primaryDNS := "1.1.1.1" // 🎯 FIXED: Use a hardcoded DNS to prevent OS command injection from malicious server responses

	switch runtime.GOOS {
	case "linux":
		cmds = append(cmds, exec.Command("ip", "link", "set", "dev", interfaceName, "up"))
		cmds = append(cmds, exec.Command("ip", "address", "add", assignedIP+"/24", "dev", interfaceName))

		// 🎯 DYNAMIC DNS (Linux systemd-resolved)
		cmds = append(cmds, exec.Command("resolvectl", "dns", interfaceName, primaryDNS))
		cmds = append(cmds, exec.Command("resolvectl", "domain", interfaceName, "~."))

		if useGlobalRouting {
			// 🛡️ THE ROUTING LOOP FIX (Linux)
			gwOut, _ := exec.Command("sh", "-c", "ip route show default | awk '/default/ {print $3}'").Output()
			gateway := strings.TrimSpace(string(gwOut))
			if gateway != "" {
				log.Printf("🛡️ Whitelisting Server IP %s via local gateway %s", serverIP, gateway)
				cmds = append(cmds, exec.Command("ip", "route", "add", serverIP, "via", gateway))
			}

			cmds = append(cmds, exec.Command("ip", "route", "add", "0.0.0.0/1", "dev", interfaceName))
			cmds = append(cmds, exec.Command("ip", "route", "add", "128.0.0.0/1", "dev", interfaceName))
		} else {
			// 🎯 DYNAMIC SPLIT TUNNEL (Linux)
			cmds = append(cmds, exec.Command("ip", "route", "add", baseSubnet, "dev", interfaceName))
		}

	case "windows":
		cmds = append(cmds, exec.Command("netsh", "interface", "ipv4", "set", "address",
			fmt.Sprintf("name=\"%s\"", interfaceName), "static", assignedIP, "255.255.255.0", "none"))

		// 🎯 THE FIX: Force Windows OS to respect the 1280 MTU!
		// This prevents the "fast ping, slow web page" TCP drop issue.
		cmds = append(cmds, exec.Command("netsh", "interface", "ipv4", "set", "subinterface",
			fmt.Sprintf("\"%s\"", interfaceName), "mtu=1280", "store=active"))

		// 🎯 METRIC PRIORITY (Force Windows to prefer the VPN)
		cmds = append(cmds, exec.Command("netsh", "interface", "ipv4", "set", "interface",
			fmt.Sprintf("name=\"%s\"", interfaceName), "metric=1"))

		// 🎯 DYNAMIC DNS (Windows)
		cmds = append(cmds, exec.Command("netsh", "interface", "ipv4", "set", "dnsservers",
			fmt.Sprintf("name=\"%s\"", interfaceName), "static", primaryDNS, "primary"))

		if useGlobalRouting {
			// 🛡️ THE ROUTING LOOP FIX (Windows)
			gwOut, _ := exec.Command("powershell", "-Command", "(Get-NetRoute -DestinationPrefix '0.0.0.0/0' | Sort-Object RouteMetric | Select-Object -First 1).NextHop").Output()
			gateway := strings.TrimSpace(string(gwOut))

			if gateway != "" {
				log.Printf("🛡️ Whitelisting Server IP %s via local gateway %s", serverIP, gateway)
				cmds = append(cmds, exec.Command("route", "add", serverIP, "mask", "255.255.255.255", gateway))
			}

			cmds = append(cmds, exec.Command("netsh", "interface", "ipv4", "add", "route", "0.0.0.0/1", interfaceName, "metric=1", "store=active"))
			cmds = append(cmds, exec.Command("netsh", "interface", "ipv4", "add", "route", "128.0.0.0/1", interfaceName, "metric=1", "store=active"))
		} else {
			// 🎯 DYNAMIC SPLIT TUNNEL (Windows)
			cmds = append(cmds, exec.Command("netsh", "interface", "ipv4", "add", "route", baseSubnet, interfaceName, "metric=1", "store=active"))
		}

	case "darwin":
		cmds = append(cmds, exec.Command("ifconfig", interfaceName, assignedIP, assignedIP, "up"))

		if useGlobalRouting {
			// 🛡️ THE ROUTING LOOP FIX (macOS)
			gwOut, _ := exec.Command("sh", "-c", "route -n get default | grep gateway | awk '{print $2}'").Output()
			gateway := strings.TrimSpace(string(gwOut))
			if gateway != "" {
				log.Printf("🛡️ Whitelisting Server IP %s via local gateway %s", serverIP, gateway)
				cmds = append(cmds, exec.Command("route", "-n", "add", "-host", serverIP, gateway))
			}

			cmds = append(cmds, exec.Command("route", "-n", "add", "-net", "0.0.0.0/1", "-interface", interfaceName))
			cmds = append(cmds, exec.Command("route", "-n", "add", "-net", "128.0.0.0/1", "-interface", interfaceName))
		} else {
			// 🎯 DYNAMIC SPLIT TUNNEL (macOS)
			cmds = append(cmds, exec.Command("route", "-n", "add", "-net", baseSubnet, "-interface", interfaceName))
		}

	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}

	// Execute all commands
	for _, cmd := range cmds {
		output, err := cmd.CombinedOutput()
		if err != nil {
			// Ignore "route already exists" errors on Windows/Mac
			if !strings.Contains(string(output), "already exists") && !strings.Contains(string(output), "File exists") {
				return fmt.Errorf("OS command failed: %s | output: %s | error: %v", cmd.String(), string(output), err)
			}
		}
	}
	return nil
}

// cleanupGlobalRoutes acts as a fail-safe sweeper.
// It deletes the VPS whitelist route and clears the DNS traps.
func (e *Engine) cleanupGlobalRoutes() {
	interfaceName := "claviger0"
	if runtime.GOOS == "darwin" {
		interfaceName = "utun"
	}

	// 🛠 Helper function to execute commands with logging and a strict timeout
	runCmd := func(name string, args ...string) {
		// 1. Give each command a maximum of 3 seconds to complete
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, name, args...)
		log.Printf("🧹 Sweeper Executing: %s %v", name, args)

		// 2. Capture both standard output and standard error
		output, err := cmd.CombinedOutput()

		// 3. Handle Timeouts
		if ctx.Err() == context.DeadlineExceeded {
			log.Printf("🚨 Sweeper Command TIMED OUT (hung): %s %v", name, args)
			return
		}

		// 4. Handle Errors (We expect some if routes don't exist, so we don't crash)
		if err != nil {
			log.Printf("ℹ️ Sweeper Command skipped/failed (Expected if route missing): %v | Output: %s", err, string(output))
		} else {
			log.Printf("✅ Sweeper Command Succeeded: %s %v", name, args)
		}
	}

	switch runtime.GOOS {
	case "linux":
		// 1. Revert DNS settings back to default
		runCmd("resolvectl", "revert", interfaceName)

		// 2. Remove the Global Routes
		runCmd("ip", "route", "del", "0.0.0.0/1")
		runCmd("ip", "route", "del", "128.0.0.0/1")

		// 3. Remove the Server Whitelist Route
		if e.activeServerIP != "" {
			runCmd("ip", "route", "del", e.activeServerIP)
		}

	case "windows":
		// 1. Reset DNS back to DHCP (Automatic) for the adapter
		runCmd("netsh", "interface", "ipv4", "set", "dnsservers",
			fmt.Sprintf("name=\"%s\"", interfaceName), "source=dhcp")

		// 2. Remove the Global Routes
		runCmd("netsh", "interface", "ipv4", "delete", "route", "0.0.0.0/1", interfaceName)
		runCmd("netsh", "interface", "ipv4", "delete", "route", "128.0.0.0/1", interfaceName)

		// 3. Remove the Server Whitelist Route from the physical gateway
		if e.activeServerIP != "" {
			runCmd("route", "delete", e.activeServerIP, "mask", "255.255.255.255")
		}

	case "darwin":
		// 1. Remove the Global Routes
		runCmd("route", "-n", "delete", "-net", "0.0.0.0/1")
		runCmd("route", "-n", "delete", "-net", "128.0.0.0/1")

		// 2. Remove the Server Whitelist Route
		if e.activeServerIP != "" {
			runCmd("route", "-n", "delete", "-host", e.activeServerIP)
		}
	}
}

// ==========================================
// TUNNEL LIFECYCLE CONTROLS
// ==========================================

// hotSwapOSDNS changes the DNS servers on the live TUN interface
func (e *Engine) hotSwapOSDNS(interfaceName, newDNS string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "windows":
		// Windows uses netsh to change DNS dynamically
		cmd = exec.Command("netsh", "interface", "ipv4", "set", "dnsservers",
			fmt.Sprintf("name=\"%s\"", interfaceName),
			"source=static",
			fmt.Sprintf("address=\"%s\"", newDNS))

	case "linux":
		// Linux relies on resolvectl (systemd-resolved) for dynamic TUN DNS
		cmd = exec.Command("resolvectl", "dns", interfaceName, newDNS)

	case "darwin":
		// macOS uses networksetup
		cmd = exec.Command("networksetup", "-setdnsservers", interfaceName, newDNS)
	}

	if cmd != nil {
		return cmd.Run()
	}

	return fmt.Errorf("unsupported operating system for DNS hot-swap")
}

func (e *Engine) HotSwapEndpoint(serverPubKeyBase64, newEndpoint, newDNS, interfaceName string) error {
	e.stateMutex.RLock()
	defer e.stateMutex.RUnlock()

	if e.wgDevice == nil {
		return fmt.Errorf("tunnel is not currently running")
	}

	// ==========================================
	// 1. DECODE THE KEY
	// ==========================================
	pubKeyBytes, err := base64.StdEncoding.DecodeString(serverPubKeyBase64)
	if err != nil {
		return fmt.Errorf("invalid base64 public key: %v", err)
	}
	hexKey := hex.EncodeToString(pubKeyBytes)

	// ==========================================
	// 2. 🎯 RESOLVE HOSTNAME TO RAW IP
	// ==========================================
	// WireGuard UAPI strictly requires IP addresses, not domain names!
	resolvedAddr, err := net.ResolveUDPAddr("udp", newEndpoint)
	if err != nil {
		return fmt.Errorf("failed to resolve endpoint hostname '%s': %v", newEndpoint, err)
	}

	// ==========================================
	// 3. INJECT WIREGUARD (Layer 3 Routing)
	// ==========================================
	// We pass the resolved IP string (e.g. "198.51.100.5:51820") instead of the domain
	uapiConfig := fmt.Sprintf("public_key=%s\nendpoint=%s\n", hexKey, resolvedAddr.String())

	err = e.wgDevice.IpcSet(uapiConfig)
	if err != nil {
		return fmt.Errorf("failed to inject new endpoint: %v", err)
	}

	e.activeServerIP = newEndpoint // You can safely store the domain here for tracking

	// ==========================================
	// 4. UPDATE OPERATING SYSTEM (DNS Layer)
	// ==========================================
	if newDNS != "" {
		log.Printf("🔄 Hot-swapping OS DNS to %s on interface %s", newDNS, interfaceName)
		err = e.hotSwapOSDNS(interfaceName, newDNS)
		if err != nil {
			log.Printf("⚠️ Warning: Endpoint updated, but DNS hot-swap failed: %v", err)
			// We don't return an error here because the tunnel itself still works!
		}
	}

	return nil
}

// Connect creates a virtual interface and configures WireGuard entirely in memory
// 🎯 UPDATED: Now accepts a specific ServerProfile and routing preference!
func (e *Engine) Connect(vault *config.ClientVault, profile *config.ServerProfile, useGlobalRouting bool) error {
	if e.GetState() != StateDisconnected {
		return fmt.Errorf("VPN is already connected or attempting to connect")
	}

	e.setState(StateConnecting)

	// Log the name of the server we are connecting to
	log.Printf("🚀 Starting Embedded Claviger Engine for: %s...", profile.Name)

	log.Println("🧹 Running Pre-Flight Sweeper to destroy any Zombie Adapters...")
	e.cleanupGlobalRoutes()

	// 🎯 Resolve the domain to a raw IP address using the PROFILE's endpoint
	udpAddr, err := net.ResolveUDPAddr("udp", profile.ServerEndpoint)
	if err != nil {
		e.setState(StateDisconnected)
		return fmt.Errorf("failed to resolve server endpoint (%s): %v", profile.ServerEndpoint, err)
	}
	resolvedEndpoint := udpAddr.String()   // e.g., "46.225.66.35:51820"
	e.activeServerIP = udpAddr.IP.String() // Save the raw IP so the sweeper can delete the route later

	interfaceName := "claviger0"
	if runtime.GOOS == "darwin" {
		interfaceName = "utun"
	}

	tunDevice, err := tun.CreateTUN(interfaceName, 1280) // device.DefaultMTU
	if err != nil {
		e.setState(StateDisconnected)
		return fmt.Errorf("failed to create TUN device: %v", err)
	}

	logger := device.NewLogger(device.LogLevelVerbose, "claviger: ")
	e.wgDevice = device.NewDevice(tunDevice, conn.NewDefaultBind(), logger)

	// 🎯 Parse keys from the PROFILE
	privKey, _ := wgtypes.ParseKey(profile.PrivateKey)
	pubKey, _ := wgtypes.ParseKey(profile.ServerKey)

	var allowedIPsBlock string
	if useGlobalRouting {
		allowedIPsBlock = "allowed_ip=0.0.0.0/0\nallowed_ip=::/0"
	} else {
		// 🎯 DYNAMIC SPLIT TUNNEL: Use the profile's BaseSubnet instead of hardcoding!
		subnet := profile.BaseSubnet
		if subnet == "" {
			subnet = "10.8.0.0/24" // Safe fallback
		}
		allowedIPsBlock = fmt.Sprintf("allowed_ip=%s", subnet)
	}

	// 🎯 USE THE RESOLVED IP HERE
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
		resolvedEndpoint, // 👈 Inject the resolved raw IP here!
		allowedIPsBlock,
	)

	if err := e.wgDevice.IpcSet(uapiConfig); err != nil {
		e.wgDevice.Close()
		e.setState(StateDisconnected)
		return fmt.Errorf("failed to configure memory device: %v", err)
	}
	e.wgDevice.Up()

	realInterfaceName, _ := tunDevice.Name()

	// 🎯 PASS THE PROFILE DATA TO THE ROUTING ENGINE
	if err := assignOSTunnelIP(realInterfaceName, profile.AssignedIP, profile.DNS, profile.BaseSubnet, useGlobalRouting, resolvedEndpoint); err != nil {
		e.wgDevice.Close()
		e.setState(StateDisconnected)
		return fmt.Errorf("failed to route OS traffic: %v", err)
	}

	e.setState(StateVerifying)
	e.startWatchdog()

	syncCtx, syncCancel := context.WithCancel(context.Background())
	e.syncCancel = syncCancel

	log.Println("Sync Start has been initiated.")
	controller.StartSyncManager(syncCtx, vault, e)

	return nil
}

// Disconnect gracefully destroys the memory interface and cleans up OS routes
func (e *Engine) Disconnect() error {
	if e.GetState() == StateDisconnected {
		return nil
	}

	log.Println("🛑 Shutting down Embedded VPN tunnel...")

	if e.syncCancel != nil {
		e.syncCancel()
		e.syncCancel = nil
	}

	// 1. Kill the Watchdog goroutine FIRST so it stops trying to reconnect
	if e.watchdogCancel != nil {
		e.watchdogCancel()
	}

	// 2. THE SWEEPER: Clean up OS routing and DNS *BEFORE* destroying the interface!
	log.Println("🧹 Sweeping OS routing tables and resetting DNS...")
	e.cleanupGlobalRoutes()

	// 3. Destroy the memory interface (This automatically drops the Split Tunnel subnets)
	if e.wgDevice != nil {
		e.wgDevice.Close()
		e.wgDevice = nil
	}

	// 4. Tell the UI we are fully shut down
	e.setState(StateDisconnected)

	log.Println("✅ Disconnected. Network routes and DNS restored to normal.")
	return nil
}

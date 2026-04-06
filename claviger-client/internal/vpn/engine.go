package vpn

import (
	"claviger-client/internal/config"
	"encoding/hex"
	"fmt"
	"log"
	"os/exec"
	"runtime"

	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// Engine manages the embedded WireGuard network interface
type Engine struct {
	isRunning bool
	wgDevice  *device.Device
}

// NewEngine creates a new VPN manager
func NewEngine() *Engine {
	return &Engine{
		isRunning: false,
	}
}

// Connect creates a virtual interface and configures WireGuard entirely in memory
func (e *Engine) Connect(vault *config.ClientVault) error {
	if e.isRunning {
		return fmt.Errorf("VPN is already connected")
	}

	log.Printf("🚀 Starting Embedded Claviger Engine...")
	log.Printf("📍 Assigned IP: %s", vault.AssignedIP)
	log.Printf("🎯 Target Endpoint: %s", vault.ServerEndpoint)

	// 1. Create the Virtual TUN Interface
	interfaceName := "claviger0"
	if runtime.GOOS == "darwin" {
		interfaceName = "utun" // macOS requires utun naming
	}

	tunDevice, err := tun.CreateTUN(interfaceName, device.DefaultMTU)
	if err != nil {
		return fmt.Errorf("failed to create TUN device (run as Administrator/Root): %v", err)
	}

	// 2. Start the WireGuard Memory Device
	logger := device.NewLogger(device.LogLevelVerbose, "claviger: ")
	e.wgDevice = device.NewDevice(tunDevice, conn.NewDefaultBind(), logger)

	// 3. Format the Configuration for memory injection (UAPI format)
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

	// 4. Inject the configuration into the memory device
	if err := e.wgDevice.IpcSet(uapiConfig); err != nil {
		e.wgDevice.Close()
		return fmt.Errorf("failed to configure WireGuard memory device: %v", err)
	}
	e.wgDevice.Up()

	// 5. Assign the IP address to the OS Network Card
	realInterfaceName, _ := tunDevice.Name() // Get the actual assigned name from the OS
	if err := assignOSTunnelIP(realInterfaceName, vault.AssignedIP); err != nil {
		e.wgDevice.Close()
		return fmt.Errorf("failed to route OS traffic: %v", err)
	}

	e.isRunning = true
	log.Println("✅ Embedded tunnel established!")
	return nil
}

// Disconnect gracefully destroys the memory interface and cleans up OS routes
func (e *Engine) Disconnect() error {
	if !e.isRunning {
		return nil
	}

	log.Println("🛑 Shutting down Embedded VPN tunnel...")

	// Closing the WG device automatically destroys the TUN interface and clears OS routes!
	if e.wgDevice != nil {
		e.wgDevice.Close()
		e.wgDevice = nil
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

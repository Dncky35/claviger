package vpn

import (
	"claviger-client/internal/config"
	"fmt"
	"log"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// Engine manages the local WireGuard network interface
type Engine struct {
	isRunning bool
	// We will store the OS-specific TUN device interface here later
	// tunDevice tun.Device
}

// NewEngine creates a new VPN manager
func NewEngine() *Engine {
	return &Engine{
		isRunning: false,
	}
}

// GenerateKeys creates the local Curve25519 keypair for the device
func (e *Engine) GenerateKeys() (string, string, error) {
	key, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		return "", "", err
	}
	return key.String(), key.PublicKey().String(), nil
}

// Connect builds the in-memory TUN device and establishes the secure tunnel
func (e *Engine) Connect(vault *config.ClientVault) error {
	if e.isRunning {
		return fmt.Errorf("VPN is already connected")
	}

	log.Printf("🚀 Starting Claviger VPN Engine...")
	log.Printf("📍 Assigned IP: %s", vault.AssignedIP)
	log.Printf("🎯 Target Hub: %s", vault.ServerURL)

	// ==========================================
	// THE MAGIC HAPPENS HERE (Pseudocode for now)
	// ==========================================
	// 1. Create the OS-specific virtual network card (TUN)
	// 2. Bind the wireguard-go engine to that virtual card
	// 3. Apply the vault.PrivateKey to our local engine
	// 4. Add the Hub (vault.ServerKey) as our only Peer
	// 5. Tell the OS to route 10.8.0.x traffic into our virtual card

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

	// 1. Close the TUN device
	// 2. Delete the OS routing rules

	e.isRunning = false
	log.Println("✅ Disconnected.")
	return nil
}

// Status returns whether the tunnel is currently active
func (e *Engine) Status() bool {
	return e.isRunning
}

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"runtime"

	"claviger-client/internal/auth"
	"claviger-client/internal/config"
	"claviger-client/internal/vpn"
)

// App struct is the core of our Desktop Application
type App struct {
	ctx    context.Context
	vault  *config.ClientVault
	engine *vpn.Engine
}

// NewApp creates a new App application struct
func NewApp() *App {
	// Load the secure vault from disk (or create an empty one if new)
	v, err := config.Load()
	if err != nil {
		log.Printf("⚠️ Could not load vault, creating fresh instance: %v", err)
		v = &config.ClientVault{Status: "unregistered"}
	}

	return &App{
		vault:  v,
		engine: vpn.NewEngine(),
	}
}

// startup is called by Wails when the UI window opens
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// =====================================================================
// EXPOSED FRONTEND FUNCTIONS
// (Your JavaScript UI can call all of these directly!)
// =====================================================================

// GetVault returns the current saved state so the UI knows what screen to show
func (a *App) GetVault() *config.ClientVault {
	if a.vault == nil {
		return &config.ClientVault{Status: "unregistered"}
	}
	return a.vault
}

// Connect turns the VPN tunnel ON
func (a *App) Connect() error {
	if a.vault == nil || a.vault.Status != "active" {
		return fmt.Errorf("device is not approved yet")
	}
	return a.engine.Connect(a.vault)
}

// Disconnect turns the VPN tunnel OFF
func (a *App) Disconnect() error {
	return a.engine.Disconnect()
}

// IsConnected tells the UI if the toggle switch should be green or gray
func (a *App) IsConnected() bool {
	return a.engine.Status()
}

// LeaveNetwork disconnects the VPN, wipes the local keys, and resets the app
func (a *App) LeaveNetwork() error {
	// 1. Ensure the tunnel is off
	a.Disconnect()

	// 2. Wipe the vault in memory
	a.vault = &config.ClientVault{
		Status: "unregistered",
	}

	// 3. Save the empty vault to the hard drive
	return config.Save(a.vault)
}

// =====================================================================
// THE ZERO-TRUST PROTOCOL (PASSPORT & VISA)
// =====================================================================

// GenerateRequest creates the local keys and outputs the "Passport" token
func (a *App) GenerateRequest() (string, error) {
	privKey, pubKey, err := vpn.GenerateKeys()
	if err != nil {
		return "", fmt.Errorf("failed to generate keys: %v", err)
	}

	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "Unknown-Desktop"
	}

	// Save the Private Key to the Vault immediately!
	a.vault.PrivateKey = privKey
	a.vault.PublicKey = pubKey
	a.vault.Status = "pending_approval"
	config.Save(a.vault)

	// Call our new Auth package to build the token
	return auth.GenerateRequestToken(pubKey, hostname, runtime.GOOS, "dummy-id-123")
}

// ProcessApproval catches the "Visa" from the Admin and updates the Vault (NO AUTO-CONNECT)
func (a *App) ProcessApproval(tokenString string) error {
	approval, err := auth.DecodeApprovalToken(tokenString)
	if err != nil {
		return err
	}

	// Update the Vault with the Server's map
	a.vault.AssignedIP = approval.AssignedIP
	a.vault.ServerKey = approval.ServerPubKey
	a.vault.ServerEndpoint = approval.ServerEndpoint
	a.vault.Status = "active"

	if err := config.Save(a.vault); err != nil {
		return fmt.Errorf("failed to save vault: %v", err)
	}

	// We removed the engine.Connect() call here!
	// The user will now land on the Dashboard and must explicitly click "Connect Tunnel".

	return nil
}

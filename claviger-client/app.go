package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"runtime"

	"claviger-client/internal/api"
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
	return a.vault
}

// Enroll asks the server to join the network
func (a *App) Enroll(serverURL, token string) error {
	// 1. Generate local cryptographic keys
	privKey, pubKey, err := a.engine.GenerateKeys()
	if err != nil {
		return fmt.Errorf("failed to generate keys: %v", err)
	}

	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "Unknown-Desktop"
	}

	// 2. Prepare the payload
	payload := api.EnrollPayload{
		Token:     token,
		PublicKey: pubKey,
		Name:      hostname,
		Platform:  runtime.GOOS,   // "windows", "darwin" (mac), or "linux"
		DeviceID:  "dummy-id-123", // Future update: fetch real hardware UUID
	}

	// 3. Send to Server
	resp, err := api.RequestEnrollment(serverURL, payload)
	if err != nil {
		return err
	}

	// 4. Save to Vault
	a.vault.ServerURL = serverURL
	a.vault.ClientID = resp.ClientID
	a.vault.PrivateKey = privKey
	a.vault.PublicKey = pubKey
	a.vault.Status = resp.Status

	// If Admin bypassed, the server instantly gives us our IP and Keys!
	if resp.Status == "active" {
		a.vault.AssignedIP = resp.AssignedIP
		a.vault.ServerKey = resp.ServerPubKey
	}

	// Securely save the file to disk
	return config.Save(a.vault)
}

// CheckApproval pings the server every 3 seconds while in the waiting room
func (a *App) CheckApproval() (string, error) {
	if a.vault.ClientID == "" || a.vault.ServerURL == "" {
		return "", fmt.Errorf("no client ID or server URL found")
	}

	resp, err := api.CheckStatus(a.vault.ServerURL, a.vault.ClientID)
	if err != nil {
		return "error", err
	}

	// If the admin approved us, update the vault with our new IP and Keys!
	if resp.Status == "active" && a.vault.Status != "active" {
		a.vault.Status = "active"
		a.vault.AssignedIP = resp.AssignedIP
		a.vault.ServerKey = resp.ServerPubKey
		config.Save(a.vault)
	}

	// If the admin rejected/deleted us, reset the vault
	if resp.Status == "rejected" {
		a.vault.Status = "unregistered"
		config.Save(a.vault)
	}

	return resp.Status, nil
}

// Connect turns the VPN tunnel ON
func (a *App) Connect() error {
	if a.vault.Status != "active" {
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

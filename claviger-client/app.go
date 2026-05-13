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

	"github.com/getlantern/systray"
	"github.com/google/uuid"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
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

	// ---------------------------------------------------------
	// THE EVENT EMITTER HOOK
	// We pass a callback to the Engine. Whenever the Watchdog changes
	// the connection state, this instantly fires a Wails Event to JavaScript!
	// ---------------------------------------------------------
	a.engine.SetStateCallback(func(newState string) {
		wailsRuntime.EventsEmit(ctx, "vpn-state-change", newState)
	})

	// ---------------------------------------------------------
	// SYSTEM TRAY BOOT
	// Boot the tray in the background so it survives window closures
	// ---------------------------------------------------------
	go systray.Run(a.onTrayReady, a.onTrayExit)
}

// =====================================================================
// SYSTEM TRAY MANAGEMENT
// =====================================================================

// onTrayReady builds the right-click menu for the taskbar icon
func (a *App) onTrayReady() {
	systray.SetIcon(trayIcon) // Uses the embedded png from main.go
	systray.SetTitle("Claviger")
	systray.SetTooltip("Claviger Client")

	mShow := systray.AddMenuItem("Show Dashboard", "Open the Claviger interface")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit Claviger", "Completely shut down the VPN and exit")

	// Listen for clicks in the background
	go func() {
		for {
			select {
			case <-mShow.ClickedCh:
				// Bring the UI back to the screen
				wailsRuntime.WindowShow(a.ctx)
			case <-mQuit.ClickedCh:
				// Ensure we disconnect gracefully before quitting
				a.engine.Disconnect()
				systray.Quit()
				wailsRuntime.Quit(a.ctx)
				os.Exit(0)
			}
		}
	}()
}

// onTrayExit handles cleanup if the tray closes natively
func (a *App) onTrayExit() {
	// Background cleanup
}

// =====================================================================
// EXPOSED FRONTEND FUNCTIONS
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

// GetTunnelState tells the UI exactly what the engine is doing
// Returns: "disconnected", "connecting", "verifying", "secured", or "reconnecting"
func (a *App) GetTunnelState() string {
	return a.engine.GetState()
}

// ToggleGlobalRouting updates the user's routing preference in the secure vault
func (a *App) ToggleGlobalRouting(isEnabled bool) error {
	if a.vault == nil {
		return fmt.Errorf("vault is not initialized")
	}

	a.vault.UseGlobalRouting = isEnabled

	// Save immediately so it survives app restarts
	if err := config.Save(a.vault); err != nil {
		return fmt.Errorf("failed to save routing preference: %v", err)
	}

	log.Printf("🌐 Global Routing preference updated: %v", isEnabled)
	return nil
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

	// 1. Generate a permanent Device ID if one doesn't exist yet
	if a.vault.DeviceID == "" {
		a.vault.DeviceID = uuid.New().String()
	}

	// 2. Save the Private Key, Public Key, and Device ID to the Vault immediately
	a.vault.PrivateKey = privKey
	a.vault.PublicKey = pubKey
	a.vault.Status = "pending_approval"

	if err := config.Save(a.vault); err != nil {
		return "", fmt.Errorf("failed to save vault: %v", err)
	}

	// 3. Call the Auth package to build the token using the real, persistent UUID
	return auth.GenerateRequestToken(pubKey, hostname, runtime.GOOS, a.vault.DeviceID)
}

// ProcessApproval catches the "Visa" from the Admin and updates the Vault
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

	// 🎯 NEW: Capture the network identity provided by the server
	a.vault.DNS = approval.DNS
	a.vault.BaseSubnet = approval.BaseSubnet

	if err := config.Save(a.vault); err != nil {
		return fmt.Errorf("failed to save vault: %v", err)
	}

	return nil
}

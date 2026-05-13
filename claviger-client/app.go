package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"

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
		v = &config.ClientVault{
			Profiles: make(map[string]*config.ServerProfile),
		}
	}

	return &App{
		vault:  v,
		engine: vpn.NewEngine(),
	}
}

// startup is called by Wails when the UI window opens
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// THE EVENT EMITTER HOOK
	a.engine.SetStateCallback(func(newState string) {
		wailsRuntime.EventsEmit(ctx, "vpn-state-change", newState)
	})

	// SYSTEM TRAY BOOT
	go systray.Run(a.onTrayReady, a.onTrayExit)
}

// =====================================================================
// SYSTEM TRAY MANAGEMENT
// =====================================================================

func (a *App) onTrayReady() {
	systray.SetIcon(trayIcon) // Uses the embedded png from main.go
	systray.SetTitle("Claviger")
	systray.SetTooltip("Claviger Client")

	mShow := systray.AddMenuItem("Show Dashboard", "Open the Claviger interface")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit Claviger", "Completely shut down the VPN and exit")

	go func() {
		for {
			select {
			case <-mShow.ClickedCh:
				wailsRuntime.WindowShow(a.ctx)
			case <-mQuit.ClickedCh:
				a.engine.Disconnect()
				systray.Quit()
				wailsRuntime.Quit(a.ctx)
				os.Exit(0)
			}
		}
	}()
}

func (a *App) onTrayExit() {
	// Background cleanup
}

// =====================================================================
// EXPOSED FRONTEND FUNCTIONS (Tunnel Controls)
// =====================================================================

// GetVault returns the current saved state so the UI knows what screen to show
func (a *App) GetVault() *config.ClientVault {
	if a.vault == nil {
		return &config.ClientVault{Profiles: make(map[string]*config.ServerProfile)}
	}
	return a.vault
}

// Connect turns the VPN tunnel ON using the currently active profile
func (a *App) Connect() error {
	if a.vault == nil || a.vault.ActiveProfileID == "" {
		return fmt.Errorf("no server selected")
	}

	profile, exists := a.vault.Profiles[a.vault.ActiveProfileID]
	if !exists || profile.Status != "active" {
		return fmt.Errorf("selected server is not approved yet")
	}

	// 🎯 Note: We now pass the specific PROFILE and the GLOBAL ROUTING flag
	return a.engine.Connect(profile, a.vault.UseGlobalRouting)
}

// Disconnect turns the VPN tunnel OFF
func (a *App) Disconnect() error {
	return a.engine.Disconnect()
}

// GetTunnelState tells the UI exactly what the engine is doing
func (a *App) GetTunnelState() string {
	return a.engine.GetState()
}

// ToggleGlobalRouting updates the user's routing preference in the secure vault
func (a *App) ToggleGlobalRouting(isEnabled bool) error {
	if a.vault == nil {
		return fmt.Errorf("vault is not initialized")
	}
	a.vault.UseGlobalRouting = isEnabled
	if err := config.Save(a.vault); err != nil {
		return fmt.Errorf("failed to save routing preference: %v", err)
	}
	return nil
}

// =====================================================================
// MULTI-SERVER MANAGEMENT (NEW FRONTEND CONTROLS)
// =====================================================================

// SetActiveProfile switches the currently selected server
func (a *App) SetActiveProfile(profileID string) error {
	if _, exists := a.vault.Profiles[profileID]; !exists {
		return fmt.Errorf("profile does not exist")
	}

	// Must disconnect from the current server before switching!
	a.Disconnect()

	a.vault.ActiveProfileID = profileID
	return config.Save(a.vault)
}

// RenameProfile allows the user to give a server a custom name
func (a *App) RenameProfile(profileID, newName string) error {
	profile, exists := a.vault.Profiles[profileID]
	if !exists {
		return fmt.Errorf("profile does not exist")
	}
	profile.Name = newName
	return config.Save(a.vault)
}

// RemoveProfile deletes a server from the vault completely
func (a *App) RemoveProfile(profileID string) error {
	if a.vault.ActiveProfileID == profileID {
		a.Disconnect()               // Disconnect if they are deleting the active server
		a.vault.ActiveProfileID = "" // Clear selection
	}

	delete(a.vault.Profiles, profileID)
	return config.Save(a.vault)
}

// =====================================================================
// THE ZERO-TRUST PROTOCOL (PASSPORT & VISA)
// =====================================================================

// GenerateRequest creates the local keys and outputs the "Passport" token for a NEW server
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

	// 2. 🎯 CREATE A NEW PROFILE IN THE MAP
	newProfileID := uuid.New().String()
	newProfile := &config.ServerProfile{
		ID:         newProfileID,
		Name:       "Pending Server...",
		PrivateKey: privKey,
		PublicKey:  pubKey,
		Status:     "pending_approval",
	}

	if a.vault.Profiles == nil {
		a.vault.Profiles = make(map[string]*config.ServerProfile)
	}

	a.vault.Profiles[newProfileID] = newProfile
	a.vault.ActiveProfileID = newProfileID // Auto-select the one we are enrolling

	if err := config.Save(a.vault); err != nil {
		return "", fmt.Errorf("failed to save vault: %v", err)
	}

	// 3. Call the Auth package to build the token
	return auth.GenerateRequestToken(pubKey, hostname, runtime.GOOS, a.vault.DeviceID)
}

// ProcessApproval catches the "Visa" from the Admin and activates the pending profile
func (a *App) ProcessApproval(tokenString string) error {
	approval, err := auth.DecodeApprovalToken(tokenString)
	if err != nil {
		return err
	}

	// 1. Find the profile we just generated the request for
	profile, exists := a.vault.Profiles[a.vault.ActiveProfileID]
	if !exists {
		return fmt.Errorf("could not find the pending profile to approve")
	}

	// 2. Update the Profile with the Server's map
	profile.AssignedIP = approval.AssignedIP
	profile.ServerKey = approval.ServerPubKey
	profile.ServerEndpoint = approval.ServerEndpoint
	profile.DNS = approval.DNS
	profile.BaseSubnet = approval.BaseSubnet
	profile.Status = "active"

	// Dynamically name the server based on its IP/Domain!
	serverIP := strings.Split(approval.ServerEndpoint, ":")[0]
	profile.Name = fmt.Sprintf("Claviger Hub (%s)", serverIP)

	if err := config.Save(a.vault); err != nil {
		return fmt.Errorf("failed to save vault: %v", err)
	}

	return nil
}

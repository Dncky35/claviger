//go:build !headless

package gui

import (
	"fmt"
	"log"
	"net"
	"os"
	"runtime"
	"strings"
	"time"

	"claviger-client/internal/auth"
	"claviger-client/internal/config"
	"claviger-client/internal/vpn"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/google/uuid"
)

func (g *ClavigerGUI) ShowEnrollmentScreen() {

	// 🎯 THE CANCEL BUTTON (Only shows if they already have an active profile)
	var fallbackID string
	canCancel := false
	for id, p := range g.Vault.Profiles {
		if p.Status == "active" {
			canCancel = true
			fallbackID = id
			break
		}
	}

	var cancelBtn *widget.Button
	if canCancel {
		cancelBtn = widget.NewButton("Cancel / Back to Dashboard", func() {
			// Revert to the last known active profile
			g.Vault.ActiveProfileID = fallbackID
			g.ActiveProfile = g.Vault.Profiles[fallbackID]
			config.Save(g.Vault)
			g.ShowDashboardScreen()
		})
	}

	title := widget.NewLabelWithStyle("Join Claviger Network", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})

	step1Label := widget.NewLabelWithStyle("Step 1: Connection Request", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	reqTokenEntry := widget.NewMultiLineEntry()
	reqTokenEntry.Wrapping = fyne.TextWrapWord
	reqTokenEntry.Disable()

	copyBtn := widget.NewButton("Copy Token", func() {
		g.Window.Clipboard().SetContent(reqTokenEntry.Text)
		dialog.ShowInformation("Copied", "Token copied to clipboard!", g.Window)
	})
	copyBtn.Disable()

	genBtn := widget.NewButton("Generate Request Token", func() {
		privKey, pubKey, _ := vpn.GenerateKeys()
		hostname, _ := os.Hostname()
		if hostname == "" {
			hostname = "Unknown-Desktop"
		}
		if g.Vault.DeviceID == "" {
			g.Vault.DeviceID = uuid.New().String()
		}

		newProfileID := uuid.New().String()
		newProfile := &config.ServerProfile{
			ID:         newProfileID,
			Name:       "Pending Server...",
			PrivateKey: privKey,
			PublicKey:  pubKey,
			Status:     "pending_approval",
		}

		if g.Vault.Profiles == nil {
			g.Vault.Profiles = make(map[string]*config.ServerProfile)
		}
		g.Vault.Profiles[newProfileID] = newProfile
		g.Vault.ActiveProfileID = newProfileID
		config.Save(g.Vault)

		token, _ := auth.GenerateRequestToken(pubKey, hostname, runtime.GOOS, g.Vault.DeviceID)
		reqTokenEntry.SetText(token)
		copyBtn.Enable()
	})

	step2Label := widget.NewLabelWithStyle("Step 2: Apply Server Approval", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	visaEntry := widget.NewEntry()
	visaEntry.SetPlaceHolder("Paste Visa Token here...")

	verifyBtn := widget.NewButton("Verify & Connect", func() {
		tokenString := strings.TrimSpace(visaEntry.Text)
		if tokenString == "" {
			dialog.ShowError(fmt.Errorf("please paste the visa token"), g.Window)
			return
		}

		// 🎯 1. ATTEMPT TO DELEGATE TO ROOT DAEMON (Linux/Mac)
		conn, err := net.DialTimeout("tcp", "127.0.0.1:42899", 2*time.Second)
		if err == nil {
			log.Println("📡 Whispering APPROVE command to root daemon...")
			payload := fmt.Sprintf("APPROVE|%s", tokenString)
			conn.Write([]byte(payload))

			// Wait for the Daemon to finish saving
			ack := make([]byte, 2)
			conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			conn.Read(ack)
			conn.Close()

			if string(ack) == "OK" {
				// The Daemon saved it! Now the GUI just reloads the file from disk.
				updatedVault, loadErr := config.Load()
				if loadErr != nil {
					dialog.ShowError(fmt.Errorf("Failed to sync with Daemon: %v", loadErr), g.Window)
					return
				}
				g.Vault = updatedVault
				g.ActiveProfile = g.Vault.Profiles[g.Vault.ActiveProfileID] // Update current memory

				dialog.ShowInformation("Success", "Device enrolled successfully via secure Daemon!", g.Window)
				g.ShowDashboardScreen()
				return
			} else {
				dialog.ShowError(fmt.Errorf("Daemon rejected the token or failed to save"), g.Window)
				return
			}
		}

		// 🎯 2. FALLBACK FOR WINDOWS (Direct Save)
		log.Println("⚠️ Daemon not found. Attempting direct save (Windows Mode)...")
		approval, err := auth.DecodeApprovalToken(tokenString)
		if err != nil {
			dialog.ShowError(fmt.Errorf("invalid Visa token: %v", err), g.Window)
			return
		}

		profile := g.Vault.Profiles[g.Vault.ActiveProfileID]
		profile.AssignedIP = approval.AssignedIP
		profile.ServerKey = approval.ServerPubKey
		profile.ServerEndpoint = approval.ServerEndpoint
		profile.DNS = approval.DNS
		profile.BaseSubnet = approval.BaseSubnet
		profile.Status = "active"
		profile.HubPort = approval.HubPort

		serverIP := strings.Split(approval.ServerEndpoint, ":")[0]
		profile.Name = fmt.Sprintf("%s", serverIP)

		if saveErr := config.Save(g.Vault); saveErr != nil {
			dialog.ShowError(fmt.Errorf("Failed to save profile: %v", saveErr), g.Window)
		} else {
			dialog.ShowInformation("Success", "Device enrolled successfully!", g.Window)
			g.ActiveProfile = profile
			g.ShowDashboardScreen()
		}
	})

	// Layout Builder
	items := []fyne.CanvasObject{
		title,
		widget.NewSeparator(),
		step1Label,
		genBtn,
		reqTokenEntry,
		copyBtn,
		widget.NewSeparator(),
		step2Label,
		visaEntry,
		verifyBtn,
	}

	// If they have the ability to cancel, inject the Cancel button at the very top!
	if canCancel {
		items = append([]fyne.CanvasObject{cancelBtn}, items...)
	}

	content := container.NewVBox(items...)
	g.Window.SetContent(content)
}

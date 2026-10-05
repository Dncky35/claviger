//go:build !headless

package gui

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"claviger-client/internal/config"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

func (g *ClavigerGUI) ShowEnrollmentScreen() {

	// 🎯 1. CANCEL BUTTON LOGIC (Determine if we can cancel)
	var fallbackID string
	canCancel := false
	for id, p := range g.Vault.Profiles {
		if p.Status == "active" {
			canCancel = true
			fallbackID = id
			break
		}
	}

	title := widget.NewLabelWithStyle("Join Claviger Network", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})

	// 🎯 2. STEP 1: CONNECTION REQUEST (Single Generate & Copy Action)
	generateAndCopyBtn := widget.NewButton("Generate & Copy Token", func() {

		// Connect to the Daemon
		conn, err := net.DialTimeout("tcp", "127.0.0.1:42899", 2*time.Second)
		if err != nil {
			dialog.ShowError(fmt.Errorf("Claviger Background Service is not running.\nPlease start the service and try again."), g.Window)
			return
		}
		defer conn.Close()

		// Ask Daemon to generate the profile and token
		conn.Write([]byte("GENERATE_TOKEN\n"))

		// Read the response from Daemon
		conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		resp, err := bufio.NewReader(conn).ReadString('\n')
		if err != nil {
			dialog.ShowError(fmt.Errorf("Failed to receive token from Daemon: %v", err), g.Window)
			return
		}

		resp = strings.TrimSpace(resp)
		parts := strings.Split(resp, "|")

		// If Daemon succeeded, it returns "OK|eyJhbGciOi..."
		if len(parts) == 2 && parts[0] == "OK" {
			token := parts[1]

			// Copy to clipboard
			g.Window.Clipboard().SetContent(token)

			// Reload the GUI's vault in memory so it sees the new pending profile
			updatedVault, _ := config.Load()
			g.Vault = updatedVault

			dialog.ShowInformation("Success", "Token generated and copied to clipboard!\nYou can now send it to your admin.", g.Window)
		} else {
			dialog.ShowError(fmt.Errorf("Daemon failed to generate token. Check daemon logs."), g.Window)
		}
	})

	// Wrap Step 1 in a Card for a cleaner look
	step1Content := container.NewVBox(
		widget.NewLabelWithStyle("Create a secure request token to send to your network admin.", fyne.TextAlignCenter, fyne.TextStyle{}),
		container.NewHBox(layout.NewSpacer(), generateAndCopyBtn, layout.NewSpacer()), // Center the button
	)
	step1Card := widget.NewCard("Step 1: Connection Request", "", step1Content)

	// 🎯 3. STEP 2: APPLY SERVER APPROVAL
	visaEntry := widget.NewEntry()
	visaEntry.SetPlaceHolder("Paste Visa Token here...")

	verifyBtn := widget.NewButton("Verify & Connect", func() {
		tokenString := strings.TrimSpace(visaEntry.Text)
		if tokenString == "" {
			dialog.ShowError(fmt.Errorf("please paste the visa token"), g.Window)
			return
		}

		// STRICT DAEMON DELEGATION (No Direct Fallback)
		conn, err := net.DialTimeout("tcp", "127.0.0.1:42899", 2*time.Second)
		if err != nil {
			log.Println("❌ Daemon not reachable:", err)
			dialog.ShowError(fmt.Errorf("Claviger Background Service is not running.\nPlease start the service from Windows Services and try again."), g.Window)
			return
		}
		defer conn.Close()

		log.Println("📡 Whispering APPROVE command to root daemon...")
		payload := fmt.Sprintf("APPROVE|%s", tokenString)
		conn.Write([]byte(payload))

		ack := make([]byte, 2)
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		conn.Read(ack)

		if string(ack) == "OK" {
			updatedVault, loadErr := config.Load()
			if loadErr != nil {
				dialog.ShowError(fmt.Errorf("Failed to sync with Daemon: %v", loadErr), g.Window)
				return
			}
			g.Vault = updatedVault
			g.ActiveProfile = g.Vault.Profiles[g.Vault.ActiveProfileID]

			dialog.ShowInformation("Success", "Device enrolled successfully via secure Daemon!", g.Window)
			g.ShowDashboardScreen()
		} else {
			dialog.ShowError(fmt.Errorf("Daemon rejected the token or failed to save"), g.Window)
		}
	})

	// Wrap Step 2 in a Card
	step2Content := container.NewVBox(
		visaEntry,
		container.NewHBox(layout.NewSpacer(), verifyBtn, layout.NewSpacer()), // Center the verify button
	)
	step2Card := widget.NewCard("Step 2: Apply Server Approval", "", step2Content)

	// 🎯 4. BUILD THE LAYOUT
	mainContent := container.NewVBox(
		title,
		widget.NewSeparator(),
		step1Card,
		step2Card,
	)

	// Pin the cancel button to the absolute bottom if applicable
	var footer fyne.CanvasObject
	if canCancel {
		cancelBtn := widget.NewButton("Cancel / Back to Dashboard", func() {
			g.Vault.ActiveProfileID = fallbackID
			g.ActiveProfile = g.Vault.Profiles[fallbackID]
			config.Save(g.Vault)
			g.ShowDashboardScreen()
		})

		// Add some padding/spacer above the cancel button
		footer = container.NewVBox(layout.NewSpacer(), cancelBtn)
	}

	// NewBorder pins items to Top, Bottom, Left, Right, Center
	// nil, footer, nil, nil = No top, Yes bottom, No left, No right.
	// mainContent fills the remaining central space.
	finalLayout := container.NewBorder(nil, footer, nil, nil, mainContent)

	g.Window.SetContent(finalLayout)
}

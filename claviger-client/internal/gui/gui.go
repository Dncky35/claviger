package gui

import (
	"log"

	"claviger-client/internal/config"
	"claviger-client/internal/vpn"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

type ClavigerGUI struct {
	Window        fyne.Window
	App           fyne.App // 🎯 NEW: Keep a reference to the Fyne App
	Vault         *config.ClientVault
	Engine        *vpn.Engine
	ActiveProfile *config.ServerProfile

	// UI Widgets
	TitleLabel   *widget.Label
	ServerSelect *widget.Select
	NameToID     map[string]string
	AddServerBtn *widget.Button
	StatusLabel  *widget.Label
	RouteCheck   *widget.Check
	ConnectBtn   *widget.Button
	RemoveBtn    *widget.Button
}

func Run(vault *config.ClientVault) {
	a := app.New()
	w := a.NewWindow("Claviger Client")

	gui := &ClavigerGUI{
		App:    a,
		Window: w,
		Vault:  vault,
		Engine: vpn.NewEngine(),
	}

	// 🎯 1. SET THE APP ICON
	// In gui.go...
	a.SetIcon(resourceIconPng)

	// 🎯 2. SYSTEM TRAY LOGIC
	if desk, ok := a.(desktop.App); ok {
		// Create the right-click menu for the system tray
		m := fyne.NewMenu("Claviger Network",
			fyne.NewMenuItem("Show Dashboard", func() {
				w.Show() // Brings the window back to the screen
			}),
			fyne.NewMenuItem("Disconnect & Quit", func() {
				a.Quit() // Completely exits the app
			}),
		)
		desk.SetSystemTrayMenu(m)
		// And in the System Tray setup...
		desk.SetSystemTrayIcon(resourceIconPng)
	}

	// 🎯 3. INTERCEPT THE "X" BUTTON
	// Instead of closing the app, we just hide the window!
	w.SetCloseIntercept(func() {
		w.Hide()
		log.Println("Window hidden to system tray.")
	})

	// The Screen Router
	if vault.ActiveProfileID == "" || len(vault.Profiles) == 0 {
		gui.ShowEnrollmentScreen()
	} else {
		gui.ActiveProfile = vault.Profiles[vault.ActiveProfileID]
		gui.ShowDashboardScreen()
	}

	w.Resize(fyne.NewSize(450, 400))
	w.CenterOnScreen()

	// ShowAndRun blocks the thread until a.Quit() is called!
	w.ShowAndRun()

	// 🎯 CLEANUP ON QUIT
	// This only fires when the user clicks "Disconnect & Quit" from the System Tray
	log.Println("⚠️ App terminating. Executing clean disconnect...")
	gui.Engine.Disconnect()
}

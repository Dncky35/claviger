//go:build !headless

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
	App           fyne.App
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

func Run(vault *config.ClientVault, wakeUpChan chan bool) {

	EnsureAdmin()

	a := app.NewWithID("com.cloudrocean.claviger-client")
	w := a.NewWindow("Claviger Zero Trust")

	// ========================================================
	// Everything below here only runs if the user IS an Admin!
	// ========================================================

	gui := &ClavigerGUI{
		App:    a,
		Window: w,
		Vault:  vault,
		Engine: vpn.NewEngine(),
	}

	a.SetIcon(resourceIconPng)

	if desk, ok := a.(desktop.App); ok {
		m := fyne.NewMenu("Claviger Network",
			fyne.NewMenuItem("Show Dashboard", func() { w.Show() }),
			fyne.NewMenuItem("Disconnect & Quit", func() { a.Quit() }),
		)
		desk.SetSystemTrayMenu(m)
		desk.SetSystemTrayIcon(resourceIconPng)
	}

	w.SetCloseIntercept(func() {
		w.Hide()
		log.Println("Window hidden to system tray.")
	})

	if vault.ActiveProfileID == "" || len(vault.Profiles) == 0 {
		gui.ShowEnrollmentScreen()
	} else {
		gui.ActiveProfile = vault.Profiles[vault.ActiveProfileID]
		gui.ShowDashboardScreen()
	}

	// 🎯 BACKGROUND LISTENER FOR INSTANCE B WAKEUP CALLS
	go func() {
		for range wakeUpChan {
			w.Show()
			w.RequestFocus() // Pulls the window to the absolute front of the screen
		}
	}()

	w.Resize(fyne.NewSize(450, 400))
	w.CenterOnScreen()
	w.ShowAndRun()

	log.Println("⚠️ App terminating. Executing clean disconnect...")
	gui.Engine.Disconnect()
}

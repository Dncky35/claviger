package gui

import (
	"log"

	"claviger-client/internal/config"
	"claviger-client/internal/vpn"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/widget"
)

type ClavigerGUI struct {
	Window        fyne.Window
	Vault         *config.ClientVault
	Engine        *vpn.Engine
	ActiveProfile *config.ServerProfile

	// UI Widgets
	TitleLabel   *widget.Label
	ServerSelect *widget.Select    // 🎯 NEW: Dropdown selector
	NameToID     map[string]string // 🎯 NEW: Maps the dropdown text to the Profile ID
	AddServerBtn *widget.Button    // 🎯 NEW: Add server button
	StatusLabel  *widget.Label
	RouteCheck   *widget.Check
	ConnectBtn   *widget.Button
	RemoveBtn    *widget.Button
}

func Run(vault *config.ClientVault) {
	a := app.New()
	w := a.NewWindow("Claviger Client")

	gui := &ClavigerGUI{
		Window: w,
		Vault:  vault,
		Engine: vpn.NewEngine(),
	}

	if vault.ActiveProfileID == "" || len(vault.Profiles) == 0 {
		gui.ShowEnrollmentScreen()
	} else {
		gui.ActiveProfile = vault.Profiles[vault.ActiveProfileID]
		gui.ShowDashboardScreen()
	}

	w.Resize(fyne.NewSize(450, 400))
	w.CenterOnScreen()
	w.ShowAndRun()

	log.Println("⚠️ UI Window closed. Executing clean disconnect...")
	gui.Engine.Disconnect()
}

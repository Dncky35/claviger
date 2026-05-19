package gui

import (
	"log"
	"os"

	"claviger-client/internal/config"
	"claviger-client/internal/vpn"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
	"golang.org/x/sys/windows" // 🎯 NEW: Required for the Windows Admin Check
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

// 🎯 NEW: Native Windows Elevation Check
func isAdmin() bool {
	var sid *windows.SID

	// Although we are using the well-known SID for the Administrators group,
	// this approach checks if the current process token actually has it enabled.
	err := windows.AllocateAndInitializeSid(
		&windows.SECURITY_NT_AUTHORITY,
		2,
		windows.SECURITY_BUILTIN_DOMAIN_RID,
		windows.DOMAIN_ALIAS_RID_ADMINS,
		0, 0, 0, 0, 0, 0,
		&sid,
	)
	if err != nil {
		return false
	}
	defer windows.FreeSid(sid)

	token := windows.Token(0)
	member, err := token.IsMember(sid)
	if err != nil {
		return false
	}
	return member
}

func Run(vault *config.ClientVault) {
	a := app.New()
	w := a.NewWindow("Claviger Zero Trust")

	// 🎯 THE GATEKEEPER CHECK: If not Admin, block the entire app!
	if !isAdmin() {
		w.SetTitle("Claviger - Permission Required")

		warningLabel := widget.NewLabelWithStyle("🛡️ Administrator Rights Required", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
		descLabel := widget.NewLabel("Claviger needs Administrator privileges to configure the virtual\nnetwork interface, manage routing tables, and secure your tunnel.\n\nPlease close the app, right-click 'claviger.exe', and select\n'Run as Administrator'.")

		exitBtn := widget.NewButton("Exit App", func() {
			os.Exit(0)
		})

		content := container.NewVBox(
			warningLabel,
			widget.NewSeparator(),
			descLabel,
			widget.NewSeparator(),
			exitBtn,
		)

		w.SetContent(content)
		w.Resize(fyne.NewSize(420, 220))
		w.CenterOnScreen()
		w.ShowAndRun()
		return // Completely halt execution here!
	}

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

	w.Resize(fyne.NewSize(450, 400))
	w.CenterOnScreen()
	w.ShowAndRun()

	log.Println("⚠️ App terminating. Executing clean disconnect...")
	gui.Engine.Disconnect()
}

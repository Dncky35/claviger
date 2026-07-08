//go:build !headless

package gui

import (
	"fmt"

	"claviger-client/internal/config"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

func (g *ClavigerGUI) ShowDashboardScreen() {
	// 🎯 INITIALIZE BINDINGS
	if g.StatusBinding == nil {
		g.StatusBinding = binding.NewString()
		g.StatusBinding.Set("Status: Disconnected")
	}
	if g.SyncStatusBinding == nil {
		g.SyncStatusBinding = binding.NewString()
		g.SyncStatusBinding.Set("Stable")
	}

	// 🎯 DYNAMIC DROPDOWN
	g.NameToID = make(map[string]string)
	var options []string
	var currentSelection string
	for id, profile := range g.Vault.Profiles {
		if profile.Status == "active" {
			displayName := fmt.Sprintf("%s (%s)", profile.Name, profile.AssignedIP)
			options = append(options, displayName)
			g.NameToID[displayName] = id
			if id == g.Vault.ActiveProfileID {
				currentSelection = displayName
			}
		}
	}

	// 🎯 CARD: STATUS & SYNC
	g.StatusLabel = widget.NewLabelWithData(g.StatusBinding)
	syncLabel := widget.NewLabelWithData(g.SyncStatusBinding)
	statusCard := widget.NewCard("Connection Status", "", container.NewVBox(
		g.StatusLabel,
		container.NewHBox(widget.NewLabel("Sync Engine:"), syncLabel),
	))

	// 🎯 CARD: SERVER MANAGEMENT
	g.ServerSelect = widget.NewSelect(options, nil)
	g.ServerSelect.SetSelected(currentSelection)
	g.ConnectBtn = widget.NewButton("Connect Tunnel", nil) // Logic in events.go

	serverCard := widget.NewCard("Active Server", "", container.NewVBox(
		g.ServerSelect,
		g.ConnectBtn,
	))

	// 🎯 INITIALIZE SETTINGS CONTROLS (Hidden until Modal opens)
	g.RouteCheck = widget.NewCheck("Enable Global Routing", func(checked bool) {
		g.Vault.UseGlobalRouting = checked
		config.Save(g.Vault)
	})
	g.RouteCheck.SetChecked(g.Vault.UseGlobalRouting)

	g.AutoStartCheck = widget.NewCheck("Enable AutoConnect", func(checked bool) {
		g.Vault.AutoConnect = checked
		config.Save(g.Vault)
	})
	g.AutoStartCheck.SetChecked(g.Vault.AutoConnect)

	g.AddServerBtn = widget.NewButton("Add New Server", g.ShowEnrollmentScreen)
	g.RemoveBtn = widget.NewButton("Remove Server", nil) // Logic in events.go

	// 🎯 SETTINGS MODAL TRIGGER
	settingsBtn := widget.NewButtonWithIcon("", theme.SettingsIcon(), func() {
		settingsContent := container.NewVBox(
			widget.NewLabelWithStyle("Network & Startup", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			g.RouteCheck,
			g.AutoStartCheck,
			widget.NewSeparator(),
			widget.NewLabelWithStyle("Server Management", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			g.AddServerBtn,
			g.RemoveBtn,
		)
		settingsDialog := dialog.NewCustom("Claviger Settings", "Close", settingsContent, g.Window)
		settingsDialog.Show()
	})

	// Wrap the settings button with a leading spacer to push it to the TOP RIGHT
	topBar := container.NewHBox(layout.NewSpacer(), settingsBtn)

	// 🎯 FINAL LAYOUT (Main View)
	content := container.NewPadded(container.NewVBox(
		topBar, // Placed at the very top
		serverCard,
		statusCard,
	))

	g.Window.SetContent(content)
	g.setupEvents()
}

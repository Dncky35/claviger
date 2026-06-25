//go:build !headless

package gui

import (
	"fmt"

	"claviger-client/internal/config"

	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
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

	// Header
	// title := widget.NewLabelWithStyle("Claviger Network", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})

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

	serverCard := widget.NewCard("Active Server", "", container.NewVBox(
		g.ServerSelect,
		g.ConnectBtn,
		g.RouteCheck,
		g.AutoStartCheck,
	))

	// 🎯 FOOTER: SETTINGS & DANGEROUS ACTION
	g.AddServerBtn = widget.NewButton("Add New Server", g.ShowEnrollmentScreen)
	g.RemoveBtn = widget.NewButton("Remove Server", nil) // Logic in events.go

	// 1. Group the server actions into a single container and hide it initially
	serverActionsContainer := container.NewVBox(
		g.RemoveBtn,
		g.AddServerBtn,
	)
	serverActionsContainer.Hide() // Hidden by default

	// 2. Create the Settings Toggle Button with a gear icon
	settingsToggleBtn := widget.NewButtonWithIcon("Settings", theme.SettingsIcon(), func() {
		// Toggle the visibility of the action buttons container
		if serverActionsContainer.Visible() {
			serverActionsContainer.Hide()
		} else {
			serverActionsContainer.Show()
		}
	})

	// Final Layout
	content := container.NewPadded(container.NewVBox(
		statusCard,
		layout.NewSpacer(),
		layout.NewSpacer(),
		serverCard,
		widget.NewSeparator(),
		settingsToggleBtn,      // The new gear button sits here
		serverActionsContainer, // This container expands/collapses when Settings is clicked
	))

	g.Window.SetContent(content)
	g.setupEvents()
}

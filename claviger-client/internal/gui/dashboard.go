//go:build !headless

package gui

import (
	"fmt"

	"claviger-client/internal/config"

	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/layout"
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

	g.ServerSelect = widget.NewSelect(options, nil)
	g.ServerSelect.SetSelected(currentSelection)
	g.ConnectBtn = widget.NewButton("Connect Tunnel", nil) // Logic in events.go
	g.RouteCheck = widget.NewCheck("Enable Global Routing", func(checked bool) {
		g.Vault.UseGlobalRouting = checked
		config.Save(g.Vault)
	})
	g.RouteCheck.SetChecked(g.Vault.UseGlobalRouting)

	// 🎯 CARD: SERVER MANAGEMENT
	serverCard := widget.NewCard("Active Server", "", container.NewVBox(
		g.ServerSelect,
		g.ConnectBtn,
		g.RouteCheck,
	))

	g.AddServerBtn = widget.NewButton("Add New Server", g.ShowEnrollmentScreen)

	// 🎯 PRIMARY ACTION: THE BIG CONNECT BUTTON

	// 🎯 CARD: STATUS & SYNC
	g.StatusLabel = widget.NewLabelWithData(g.StatusBinding)
	syncLabel := widget.NewLabelWithData(g.SyncStatusBinding)
	statusCard := widget.NewCard("Connection Status", "", container.NewVBox(
		g.StatusLabel,
		container.NewHBox(widget.NewLabel("Sync Engine:"), syncLabel),
	))

	// 🎯 FOOTER: SETTINGS & DANGEROUS ACTIONS

	g.RemoveBtn = widget.NewButton("Remove Server", nil) // Logic in events.go

	// Final Layout
	content := container.NewPadded(container.NewVBox(
		statusCard,
		layout.NewSpacer(),
		layout.NewSpacer(),
		serverCard,
		widget.NewSeparator(),
		g.RemoveBtn,
		g.AddServerBtn,
	))

	g.Window.SetContent(content)
	g.setupEvents()
}

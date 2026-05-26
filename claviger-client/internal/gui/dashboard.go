//go:build !headless

package gui

import (
	"fmt"

	"claviger-client/internal/config"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

func (g *ClavigerGUI) ShowDashboardScreen() {
	g.TitleLabel = widget.NewLabelWithStyle("Claviger Network", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})

	// 🎯 DYNAMIC DROPDOWN POPULATION
	g.NameToID = make(map[string]string)
	var options []string
	var currentSelection string

	for id, profile := range g.Vault.Profiles {
		if profile.Status == "active" {
			// Format it nicely: "Server Name (10.8.0.x)"
			displayName := fmt.Sprintf("%s (%s)", profile.Name, profile.AssignedIP)
			options = append(options, displayName)
			g.NameToID[displayName] = id

			if id == g.Vault.ActiveProfileID {
				currentSelection = displayName
			}
		}
	}

	g.ServerSelect = widget.NewSelect(options, nil) // Logic is wired in events.go
	g.ServerSelect.SetSelected(currentSelection)

	g.AddServerBtn = widget.NewButton("Add Node", nil)

	// Put the Dropdown and the Add button on the same line
	serverRow := container.NewBorder(nil, nil, nil, g.AddServerBtn, g.ServerSelect)

	g.StatusLabel = widget.NewLabel("Status: Disconnected")

	g.RouteCheck = widget.NewCheck("Enable Global Routing", func(checked bool) {
		g.Vault.UseGlobalRouting = checked
		config.Save(g.Vault)
	})
	g.RouteCheck.SetChecked(g.Vault.UseGlobalRouting)

	g.ConnectBtn = widget.NewButton("Connect Tunnel", nil)
	g.RemoveBtn = widget.NewButton("Delete Server", nil)

	content := container.NewVBox(
		g.TitleLabel,
		widget.NewSeparator(),
		widget.NewLabelWithStyle("Active Server:", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		serverRow, // 🎯 NEW: Injected the Dropdown row
		g.RouteCheck,
		widget.NewSeparator(),
		g.StatusLabel,
		g.ConnectBtn,
		g.RemoveBtn,
	)

	g.Window.SetContent(content)
	g.setupEvents()
}

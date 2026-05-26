//go:build !headless

package gui

import (
	"log"

	"claviger-client/internal/config"
	"claviger-client/internal/vpn"

	"fyne.io/fyne/v2/dialog"
)

func (g *ClavigerGUI) setupEvents() {

	updateUI := func(newState string) {
		g.StatusLabel.SetText("Status: " + newState)

		switch newState {
		case vpn.StateDisconnected:
			g.ConnectBtn.SetText("Connect Tunnel")
			g.ConnectBtn.Enable()
			g.RouteCheck.Enable()
			g.RemoveBtn.Enable()
			g.ServerSelect.Enable()
			g.AddServerBtn.Enable()

		case vpn.StateConnecting, vpn.StateVerifying:
			// 🎯 FIX 2: Do NOT disable the button. Change it to an Abort switch!
			g.ConnectBtn.SetText("Abort Connection")
			g.ConnectBtn.Enable() // KEEP ENABLED!
			g.RouteCheck.Disable()
			g.RemoveBtn.Disable()
			g.ServerSelect.Disable()
			g.AddServerBtn.Disable()

		case vpn.StateSecured:
			g.ConnectBtn.SetText("Disconnect Tunnel")
			g.ConnectBtn.Enable()
			g.RouteCheck.Disable()
			g.RemoveBtn.Disable()
			g.ServerSelect.Disable()
			g.AddServerBtn.Disable()

		case vpn.StateReconnecting:
			// 🎯 FIX 2: Allow aborting during ghost connections
			g.ConnectBtn.SetText("Ghost Connection (Abort?)")
			g.ConnectBtn.Enable() // KEEP ENABLED!
			g.RemoveBtn.Disable()
			g.ServerSelect.Disable()
			g.AddServerBtn.Disable()
		}
	}

	g.Engine.SetStateCallback(updateUI)

	g.ServerSelect.OnChanged = func(selected string) {
		if id, exists := g.NameToID[selected]; exists {
			g.Vault.ActiveProfileID = id
			g.ActiveProfile = g.Vault.Profiles[id]
			config.Save(g.Vault)
		}
	}

	g.AddServerBtn.OnTapped = func() {
		g.ShowEnrollmentScreen()
	}

	// Because we kept the button enabled during connecting states,
	// clicking it will now instantly fire g.Engine.Disconnect() !
	g.ConnectBtn.OnTapped = func() {
		state := g.Engine.GetState()
		if state == vpn.StateDisconnected {
			go func() {
				err := g.Engine.Connect(g.ActiveProfile, g.Vault.UseGlobalRouting)
				if err != nil {
					log.Printf("Connect error: %v", err)
				}
			}()
		} else {
			// This now safely aborts Connecting, Verifying, AND Secured states!
			go g.Engine.Disconnect()
		}
	}

	g.RemoveBtn.OnTapped = func() {
		dialog.ShowConfirm("Delete Server", "Are you sure you want to permanently delete this server profile?", func(confirmed bool) {
			if confirmed {
				// 1. Delete the current server from the vault
				delete(g.Vault.Profiles, g.Vault.ActiveProfileID)

				// 🎯 FIX 1: Search the vault to see if any active servers are left!
				var nextProfileID string
				for id, p := range g.Vault.Profiles {
					if p.Status == "active" {
						nextProfileID = id
						break // Found one! Stop looking.
					}
				}

				if nextProfileID != "" {
					// We have a backup server! Switch to it and stay on the Dashboard.
					g.Vault.ActiveProfileID = nextProfileID
					g.ActiveProfile = g.Vault.Profiles[nextProfileID]
					config.Save(g.Vault)

					dialog.ShowInformation("Deleted", "Server removed. Switched to next available server.", g.Window)
					g.ShowDashboardScreen() // Reload UI with new server
				} else {
					// The vault is completely empty. Fallback to Enrollment.
					g.Vault.ActiveProfileID = ""
					config.Save(g.Vault)

					dialog.ShowInformation("Deleted", "Last server removed.", g.Window)
					g.ShowEnrollmentScreen()
				}
			}
		}, g.Window)
	}

	updateUI(g.Engine.GetState())
}

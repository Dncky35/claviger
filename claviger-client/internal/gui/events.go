//go:build !headless

package gui

import (
	"fmt"
	"log"
	"net"
	"time"

	"claviger-client/internal/config"
	"claviger-client/internal/controller"
	"claviger-client/internal/vpn"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
)

func (g *ClavigerGUI) SafeUpdate(fn func()) {
	fyne.Do(fn)
}

func (g *ClavigerGUI) setupEvents() {

	updateUI := func(newState string) {
		// Update the main status label safely
		if g.StatusBinding != nil {
			g.StatusBinding.Set("Status: " + newState)
		}

		g.SafeUpdate(func() {
			switch newState {
			case vpn.StateDisconnected:
				g.ConnectBtn.SetText("Connect")
				g.ConnectBtn.Enable()
				g.RouteCheck.Enable()
				g.RemoveBtn.Enable()
				g.ServerSelect.Enable()
				g.AddServerBtn.Enable()

				// 🎯 Reset sync status when disconnected
				g.SyncStatusBinding.Set(controller.SyncStable)

			case vpn.StateConnecting, vpn.StateVerifying:
				g.ConnectBtn.SetText("Cancel")
				g.ConnectBtn.Enable()
				g.RouteCheck.Disable()
				g.RemoveBtn.Disable()
				g.ServerSelect.Disable()
				g.AddServerBtn.Disable()

				// Sync hasn't started yet during connecting

			case vpn.StateSecured:
				g.ConnectBtn.SetText("Disconnect")
				g.ConnectBtn.Enable()
				g.RouteCheck.Disable()
				g.RemoveBtn.Disable()
				g.ServerSelect.Disable()
				g.AddServerBtn.Disable()

				// 🎯 LIVE SYNC UPDATE: Fetch the exact status from the Engine!
				currentSyncStatus := g.Engine.GetSyncStatus()
				g.SyncStatusBinding.Set(currentSyncStatus)

			case vpn.StateReconnecting:
				g.ConnectBtn.SetText("Cancel")
				g.ConnectBtn.Enable()
				g.RemoveBtn.Disable()
				g.ServerSelect.Disable()
				g.AddServerBtn.Disable()
			}
		})
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

	g.ConnectBtn.OnTapped = func() {
		state := g.Engine.GetState()

		if state == vpn.StateDisconnected {
			profile := g.ActiveProfile
			routing := "split"
			if g.Vault.UseGlobalRouting {
				routing = "global"
			}

			// 🎯 ATTEMPT TO DELEGATE TO THE ROOT DAEMON
			conn, err := net.DialTimeout("tcp", "127.0.0.1:42899", 2*time.Second)
			if err == nil {
				// Success! The background daemon is running. Send the Connect command.
				log.Println("📡 Whispering CONNECT command to root daemon...")
				payload := fmt.Sprintf("CONNECT|%s|%s", profile.ID, routing)
				conn.Write([]byte(payload))
				conn.Close()

				// Update GUI State visually
				// g.Engine.SetState(vpn.StateConnecting)
			} else {
				// 🎯 FALLBACK (Usually for Windows standalone mode)
				log.Println("⚠️ Daemon not found. Attempting direct connection (requires admin)...")
				go func() {
					err := g.Engine.Connect(g.Vault, profile, g.Vault.UseGlobalRouting)
					if err != nil {
						log.Printf("Connect error: %v", err)
					}
				}()
			}
		} else {
			// 🎯 DISCONNECT LOGIC
			conn, err := net.DialTimeout("tcp", "127.0.0.1:42899", 2*time.Second)
			if err == nil {
				log.Println("📡 Whispering DISCONNECT to root daemon...")
				conn.Write([]byte("DISCON"))
				conn.Close()
			} else {
				go g.Engine.Disconnect()
			}
		}
	}

	g.RemoveBtn.OnTapped = func() {
		dialog.ShowConfirm("Delete Server", "Are you sure you want to permanently delete this server profile?", func(confirmed bool) {
			if confirmed {
				delete(g.Vault.Profiles, g.Vault.ActiveProfileID)

				var nextProfileID string
				for id, p := range g.Vault.Profiles {
					if p.Status == "active" {
						nextProfileID = id
						break
					}
				}

				if nextProfileID != "" {
					g.Vault.ActiveProfileID = nextProfileID
					g.ActiveProfile = g.Vault.Profiles[nextProfileID]
					config.Save(g.Vault)
					dialog.ShowInformation("Deleted", "Server removed. Switched to next available server.", g.Window)
					g.ShowDashboardScreen()
				} else {
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

//go:build !headless

package gui

import (
	"claviger-client/internal/config"
	"claviger-client/internal/controller"
	"claviger-client/internal/vpn"
	"fmt"
	"log"
	"net"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
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

				// ⚠️ NOTE: You will need to add a GetDaemonSyncStatus() IPC method later!
				// For now, we hardcode it to Stable so the UI doesn't crash.
				g.SyncStatusBinding.Set(controller.SyncStable)

			case vpn.StateReconnecting:
				g.ConnectBtn.SetText("Cancel")
				g.ConnectBtn.Enable()
				g.RemoveBtn.Disable()
				g.ServerSelect.Disable()
				g.AddServerBtn.Disable()
			}
		})
	}

	// -------------------------------------------------------------------
	// 🎯 THE FIX: Background Polling Loop instead of Engine Callbacks
	// -------------------------------------------------------------------
	go func() {
		lastState := ""
		for {
			// Ask the Windows Service/Linux Daemon over TCP
			currentState := g.GetDaemonState()

			// Only update the UI if the state actually changed
			if currentState != lastState {
				log.Printf("🖥️ UI detected state change: %s -> %s", lastState, currentState)
				updateUI(currentState)
				lastState = currentState
			}

			// Wait 1 second before checking again
			time.Sleep(1 * time.Second)
		}
	}()
	// -------------------------------------------------------------------

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
		state := g.GetDaemonState()

		if state == vpn.StateDisconnected || state == vpn.StateReconnecting {
			log.Println("🖥️ UI: Commanding Daemon to Connect...")
			g.SendConnectCommandToDaemon()
		} else {
			log.Println("🖥️ UI: Commanding Daemon to Disconnect...")
			g.SendDisconnectCommandToDaemon()
		}
	}

	g.RemoveBtn = widget.NewButton("Remove Server", func() {
		// Add a safety confirmation dialog
		confirmMessage := fmt.Sprintf("Are you sure you want to remove this server (%s) profile and disconnect?", g.ActiveProfile.Name)
		dialog.ShowConfirm("Remove Server", confirmMessage, func(confirm bool) {
			if !confirm {
				return
			}

			// 🎯 STRICT DAEMON DELEGATION (No Direct Fallback)
			conn, err := net.DialTimeout("tcp", "127.0.0.1:42899", 2*time.Second)
			if err != nil {
				log.Println("❌ Daemon not reachable:", err)
				dialog.ShowError(fmt.Errorf("Claviger Background Service is not running.\nPlease start the service from Windows Services and try again."), g.Window)
				return
			}
			defer conn.Close()

			log.Println("📡 Whispering REMOVE command to root daemon...")
			conn.Write([]byte("REMOVE"))

			// Wait for the Daemon to finish deleting & saving
			ack := make([]byte, 2)
			conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			conn.Read(ack)

			if string(ack) == "OK" {
				// The Daemon deleted it! Now the GUI just reloads the file from disk.
				updatedVault, loadErr := config.Load()
				if loadErr != nil {
					dialog.ShowError(fmt.Errorf("Failed to sync with Daemon: %v", loadErr), g.Window)
					return
				}
				g.Vault = updatedVault
				g.ActiveProfile = nil // Clear current memory

				dialog.ShowInformation("Success", "Server profile removed successfully!", g.Window)
				g.ShowDashboardScreen()
			} else {
				dialog.ShowError(fmt.Errorf("Daemon failed to remove the profile"), g.Window)
			}
		}, g.Window)
	})
}

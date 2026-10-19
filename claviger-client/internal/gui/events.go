//go:build !headless

package gui

import (
	"bufio"
	"claviger-client/internal/config"
	"claviger-client/internal/controller"
	"claviger-client/internal/vpn"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
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
		// 🎯 FIX: Wrap EVERY UI and binding change inside SafeUpdate/fyne.Do
		g.SafeUpdate(func() {
			// Update the main status label safely
			if g.StatusBinding != nil {
				g.StatusBinding.Set("Status: " + newState)
			}

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
				g.ConnectBtn.Enable() // Enabled so the user can abort
				g.RouteCheck.Disable()
				g.RemoveBtn.Disable()
				g.ServerSelect.Disable()
				g.AddServerBtn.Disable()

			case vpn.StateSecured:
				g.ConnectBtn.SetText("Disconnect")
				g.ConnectBtn.Enable()
				g.RouteCheck.Disable()
				g.RemoveBtn.Disable()
				g.ServerSelect.Disable()
				g.AddServerBtn.Disable()

				g.SyncStatusBinding.Set(controller.SyncStable)

			case vpn.StateReconnecting:
				g.ConnectBtn.SetText("Cancel") // Or "Disconnect"
				g.ConnectBtn.Enable()          // Enabled so the user can abort a stuck reconnect loop
				g.RouteCheck.Disable()
				g.RemoveBtn.Disable()
				g.ServerSelect.Disable()
				g.AddServerBtn.Disable()

			}
		})
	}

	// -------------------------------------------------------------------
	// 🎯 Background Polling Loop
	// -------------------------------------------------------------------
	go func() {
		lastState := ""
		lastSync := ""

		for {
			// Ask the Windows Service/Linux Daemon over TCP for both states
			currentState, currentSync := g.GetDaemonState()

			// 1. Handle Connection State changes
			if currentState != lastState {
				log.Printf("🖥️ UI detected state change: %s -> %s", lastState, currentState)
				updateUI(currentState) // Assuming this updates g.StatusBinding
				lastState = currentState
			}

			// 2. Handle Sync State changes
			if currentSync != lastSync {
				log.Printf("🔄 UI detected sync change: %s -> %s", lastSync, currentSync)
				// Immediately update the bound UI variable for sync
				g.SyncStatusBinding.Set(currentSync)
				lastSync = currentSync
			}

			// Wait 1 second before checking again
			time.Sleep(1 * time.Second)
		}
	}()
	// -------------------------------------------------------------------

	g.ServerSelect.OnChanged = func(selected string) {
		if id, exists := g.NameToID[selected]; exists {
			// 1. Update the local GUI state immediately for UI responsiveness
			g.Vault.ActiveProfileID = id
			g.ActiveProfile = g.Vault.Profiles[id]

			// 2. Delegate the actual disk save to the Daemon
			conn, err := net.DialTimeout("tcp", "127.0.0.1:42899", 2*time.Second)
			if err != nil {
				log.Println("❌ Daemon not reachable:", err)
				return
			}
			defer conn.Close()

			// Send command with \n
			payload := fmt.Sprintf("SET_ACTIVE_PROFILE|%s\n", id)
			conn.Write([]byte(payload))

			// Read response to ensure it was saved
			conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			resp, _ := bufio.NewReader(conn).ReadString('\n')
			if strings.TrimSpace(resp) != "OK" {
				log.Println("❌ Daemon failed to save the active profile change.")
			}
		}
	}

	g.AddServerBtn.OnTapped = func() {
		g.ShowEnrollmentScreen()
	}

	g.ConnectBtn.OnTapped = func() {
		state, _ := g.GetDaemonState()

		if state == vpn.StateConnecting || state == vpn.StateVerifying || state == vpn.StateReconnecting {
			log.Println("🖥️ UI: Aborting connection/reconnection attempt...")

			g.ConnectBtn.Disable()
			g.ConnectBtn.SetText("Aborting...")

			go func() {
				// Send the explicit abort/disconnect command safely
				g.SendDisconnectCommandToDaemon()
			}()
			return
		}

		if state == vpn.StateSecured {
			log.Println("🖥️ UI: Commanding Daemon to Disconnect...")
			g.ConnectBtn.Disable()
			g.ConnectBtn.SetText("Disconnecting...")

			go func() {
				g.SendDisconnectCommandToDaemon()
			}()
			return
		}

		if state == vpn.StateDisconnected {
			log.Println("🖥️ UI: Commanding Daemon to Connect...")
			g.ConnectBtn.Disable()
			g.ConnectBtn.SetText("Connecting...")

			go func() {
				g.SendConnectCommandToDaemon()
			}()
			return
		}
	}

	g.AddServerBtn = widget.NewButton("Add New Server", func() {

		if g.SettingsDialog != nil {
			g.SettingsDialog.Hide()
			g.SettingsDialog = nil
		}

		overlays := g.Window.Canvas().Overlays()
		if top := overlays.Top(); top != nil {
			overlays.Remove(top)
		}

		// cleanup before we completely replace the window content.
		time.AfterFunc(100*time.Millisecond, func() {
			g.ShowEnrollmentScreen()
		})
	})

	g.RemoveBtn = widget.NewButton("Remove Server", func() {
		confirmMessage := fmt.Sprintf("Are you sure you want to remove this server (%s) profile and disconnect?", g.ActiveProfile.Name)
		dialog.ShowConfirm("Remove Server", confirmMessage, func(confirm bool) {
			if !confirm {
				return
			}

			conn, err := net.DialTimeout("tcp", "127.0.0.1:42899", 2*time.Second)
			if err != nil {
				log.Println("❌ Daemon not reachable:", err)
				dialog.ShowError(fmt.Errorf("Claviger Background Service is not running.\nPlease start the service from Windows Services and try again."), g.Window)
				return
			}
			defer conn.Close()

			// 🎯 THE FIX: Append the exact ID to the REMOVE command
			payload := fmt.Sprintf("REMOVE|%s\n", g.ActiveProfile.ID)
			log.Printf("📡 Whispering %s command to root daemon...", payload)
			conn.Write([]byte(payload))

			conn.SetReadDeadline(time.Now().Add(3 * time.Second))
			resp, err := bufio.NewReader(conn).ReadString('\n')
			if err != nil && err != io.EOF {
				dialog.ShowError(fmt.Errorf("Failed to receive response from Daemon: %v", err), g.Window)
				return
			}

			if strings.TrimSpace(resp) == "OK" {
				// Reload vault state saved by Daemon
				updatedVault, loadErr := config.Load()
				if loadErr != nil {
					dialog.ShowError(fmt.Errorf("Failed to sync with Daemon: %v", loadErr), g.Window)
					return
				}

				g.Vault = updatedVault

				// Synchronize active profile reference in memory
				if g.Vault.ActiveProfileID != "" {
					g.ActiveProfile = g.Vault.Profiles[g.Vault.ActiveProfileID]
				} else {
					g.ActiveProfile = nil
				}

				dialog.ShowInformation("Success", "Server profile removed successfully!", g.Window)
				g.ShowDashboardScreen()
			} else {
				dialog.ShowError(fmt.Errorf("Daemon failed to remove the profile"), g.Window)
			}
		}, g.Window)
	})
}

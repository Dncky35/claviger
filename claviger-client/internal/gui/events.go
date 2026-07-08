//go:build !headless

package gui

import (
	"claviger-client/internal/config"
	"claviger-client/internal/controller"
	"claviger-client/internal/vpn"
	"log"
	"time"

	"fyne.io/fyne/v2"
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

	g.RemoveBtn.OnTapped = func() {
		// ... (Your existing RemoveBtn logic is perfect, leave it as is) ...
	}
}

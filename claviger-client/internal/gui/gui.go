//go:build !headless

package gui

import (
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"claviger-client/internal/config"
	"claviger-client/internal/vpn"

	"fyne.io/fyne/v2"

	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

type ClavigerGUI struct {
	Window        fyne.Window
	App           fyne.App
	Vault         *config.ClientVault
	ActiveProfile *config.ServerProfile

	// UI Widgets
	TitleLabel     *widget.Label
	ServerSelect   *widget.Select
	NameToID       map[string]string
	AddServerBtn   *widget.Button
	StatusLabel    *widget.Label
	RouteCheck     *widget.Check
	AutoStartCheck *widget.Check
	ConnectBtn     *widget.Button
	RemoveBtn      *widget.Button
	SettingsDialog dialog.Dialog

	StatusBinding     binding.String // Tracks "Connected/Disconnected"
	SyncStatusBinding binding.String // Tracks "Stable/Syncing"
}

// Ping the local daemon to see if it is listening
func (gui *ClavigerGUI) isDaemonAlive() bool {
	conn, err := net.DialTimeout("tcp", "127.0.0.1:42899", 1*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// Show a blocking screen that prevents user interaction until the service is restored
func (gui *ClavigerGUI) ShowServiceOfflineScreen() {
	// 🎯 Wrap the entire UI construction and injection in fyne.Do
	fyne.Do(func() {
		title := widget.NewLabelWithStyle("⚠️ Service Offline", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
		desc := widget.NewLabel("The Claviger Zero Trust background service is not reachable.")
		fixInst := widget.NewLabel("Please ensure the service is running in Windows Services or Systemd.")

		retryBtn := widget.NewButton("Retry Connection", func() {
			if gui.isDaemonAlive() {
				// If it's back online, resume normal startup flow
				gui.RouteToMainScreen()
			} else {
				dialog.ShowError(fmt.Errorf("Daemon is still unreachable"), gui.Window)
			}
		})

		content := container.NewVBox(title, desc, fixInst, retryBtn)

		gui.Window.SetContent(container.NewCenter(content))
	})
}

// Helper to handle the routing logic that used to be directly in Run()
func (gui *ClavigerGUI) RouteToMainScreen() {
	if gui.Vault.ActiveProfileID == "" || len(gui.Vault.Profiles) == 0 {
		gui.ShowEnrollmentScreen()
	} else {
		gui.ActiveProfile = gui.Vault.Profiles[gui.Vault.ActiveProfileID]
		gui.ShowDashboardScreen()

		// Only attempt auto-connect AFTER we confirm daemon is alive
		if gui.Vault.AutoConnect && gui.Vault.ActiveProfileID != "" {
			go gui.SendConnectCommandToDaemon()
		}
	}
}

func Run(vault *config.ClientVault, wakeUpChan chan bool) {
	a := app.NewWithID("com.cloudrocean.claviger-client")
	w := a.NewWindow("Claviger Client")

	gui := &ClavigerGUI{
		App:    a,
		Window: w,
		Vault:  vault,
	}

	a.SetIcon(resourceIconPng)

	if desk, ok := a.(desktop.App); ok {
		m := fyne.NewMenu("Claviger Network",
			fyne.NewMenuItem("Show Dashboard", func() { w.Show() }),
			fyne.NewMenuItem("Disconnect & Quit", func() { gui.SendDisconnectCommandToDaemon(); a.Quit() }),
		)
		desk.SetSystemTrayMenu(m)
		desk.SetSystemTrayIcon(resourceIconPng)
	}

	w.SetCloseIntercept(func() {
		w.Hide()
		log.Println("Window hidden to system tray.")
	})

	w.Resize(fyne.NewSize(450, 400))
	w.CenterOnScreen()

	// 🎯 ZERO TRUST BLOCKER: Check daemon before loading UI
	if !gui.isDaemonAlive() {
		gui.ShowServiceOfflineScreen()
	} else {
		gui.RouteToMainScreen()
	}

	// 🎯 BACKGROUND LISTENER FOR WAKEUP CALLS
	go func() {
		for range wakeUpChan {
			w.Show()
			w.RequestFocus()
		}
	}()

	w.ShowAndRun()

	log.Println("⚠️ App terminating. Executing clean disconnect...")
}

func (gui *ClavigerGUI) SendConnectCommandToDaemon() {
	profile := gui.ActiveProfile
	routing := "split"
	if gui.Vault.UseGlobalRouting {
		routing = "global"
	}

	// 1. ATTEMPT TO DELEGATE TO THE DAEMON
	conn, err := net.DialTimeout("tcp", "127.0.0.1:42899", 2*time.Second)
	if err != nil {
		// 🛑 UX FIX: Show visual error and trap them on the offline screen
		log.Printf("❌ Daemon not found: %v", err)
		dialog.ShowError(fmt.Errorf("The background service died or is unreachable."), gui.Window)
		gui.ShowServiceOfflineScreen() // Instantly lock the UI
		return
	}

	defer conn.Close()
	log.Println("📡 Whispering CONNECT command to daemon...")

	payload := fmt.Sprintf("CONNECT|%s|%s", profile.ID, routing)
	if _, writeErr := conn.Write([]byte(payload)); writeErr != nil {
		dialog.ShowError(fmt.Errorf("Failed to send command to service."), gui.Window)
		return
	}

	// 2. Await Confirmation
	buf := make([]byte, 16)
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	n, readErr := conn.Read(buf)
	if readErr != nil {
		dialog.ShowError(fmt.Errorf("Service timed out while connecting."), gui.Window)
		return
	}

	response := strings.TrimSpace(string(buf[:n]))
	if response == "OK" {
		log.Println("✅ Connection delegated successfully.")
	} else {
		dialog.ShowError(fmt.Errorf("Engine Rejected: %s", response), gui.Window)
	}
}

func (gui *ClavigerGUI) SendDisconnectCommandToDaemon() {
	// 🎯 1. ATTEMPT TO DELEGATE TO THE ROOT DAEMON
	conn, err := net.DialTimeout("tcp", "127.0.0.1:42899", 2*time.Second)
	if err != nil {
		// 🛑 UX FIX: Trap the user if the service died before they clicked disconnect
		log.Printf("❌ Daemon not found: %v", err)
		dialog.ShowError(fmt.Errorf("The background service died or is unreachable."), gui.Window)
		gui.ShowServiceOfflineScreen()
		return
	}
	defer conn.Close()

	log.Println("📡 Whispering DISCONNECT to root daemon...")

	if _, writeErr := conn.Write([]byte("DISCON")); writeErr != nil {
		dialog.ShowError(fmt.Errorf("Failed to send disconnect command to service."), gui.Window)
		return
	}

	// 🛑 CRITICAL FIX: Must wait for Ack here too, otherwise GUI might
	// refresh its state before the Daemon has actually finished tearing down!
	buf := make([]byte, 16)
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	n, readErr := conn.Read(buf)
	if readErr != nil {
		dialog.ShowError(fmt.Errorf("Service timed out while disconnecting."), gui.Window)
		return
	}

	response := strings.TrimSpace(string(buf[:n]))
	if response == "OK" {
		log.Println("✅ Disconnect acknowledged by daemon.")
	} else {
		dialog.ShowError(fmt.Errorf("Engine Rejected: %s", response), gui.Window)
	}
}

// GetDaemonState pings the background service via TCP to get the true VPN state AND Sync state.
func (gui *ClavigerGUI) GetDaemonState() (string, string) {
	// 1. Quick dial to the daemon (1-second timeout so the UI doesn't freeze!)
	conn, err := net.DialTimeout("tcp", "127.0.0.1:42899", 1*time.Second)
	if err != nil {
		gui.ShowServiceOfflineScreen()
		return vpn.StateDisconnected, "Unknown"
	}
	defer conn.Close()

	// 2. Whisper the status command
	if _, err := conn.Write([]byte("STATUS")); err != nil {
		gui.ShowServiceOfflineScreen()
		return vpn.StateDisconnected, "Unknown"
	}

	// 3. Read the exact string response from the Engine
	// Slightly increased buffer to handle the combined string "RECONNECTING|SYNCHRONIZING"
	buf := make([]byte, 64)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := conn.Read(buf)
	if err != nil {
		gui.ShowServiceOfflineScreen()
		return vpn.StateDisconnected, "Unknown"
	}

	response := strings.TrimSpace(string(buf[:n]))

	// 4. Split the response by the pipe delimiter
	parts := strings.Split(response, "|")

	connState := parts[0]
	syncState := "Unknown"
	if len(parts) > 1 {
		syncState = parts[1]
	}

	// Ensure the daemon didn't return garbage data for connection state
	switch connState {
	case vpn.StateConnecting, vpn.StateVerifying, vpn.StateSecured, vpn.StateReconnecting:
		return connState, syncState
	default:
		return vpn.StateDisconnected, syncState
	}
}

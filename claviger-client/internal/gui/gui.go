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
	"fyne.io/fyne/v2/data/binding"
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

	StatusBinding     binding.String // Tracks "Connected/Disconnected"
	SyncStatusBinding binding.String // Tracks "Stable/Syncing"
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

	if vault.ActiveProfileID == "" || len(vault.Profiles) == 0 {
		gui.ShowEnrollmentScreen()
	} else {
		gui.ActiveProfile = vault.Profiles[vault.ActiveProfileID]
		gui.ShowDashboardScreen()
	}

	// 🎯 BACKGROUND LISTENER FOR INSTANCE B WAKEUP CALLS
	go func() {
		for range wakeUpChan {
			w.Show()
			w.RequestFocus() // Pulls the window to the absolute front of the screen
		}
	}()

	w.Resize(fyne.NewSize(450, 400))
	w.CenterOnScreen()

	// 🎯 FIX: Auto-Connect via IPC, not direct Engine call!
	if vault.AutoConnect && vault.ActiveProfileID != "" {
		// This should trigger your function that sends a TCP command
		// to 127.0.0.1:42899 telling the Daemon to connect!
		go gui.SendConnectCommandToDaemon()
	}

	w.ShowAndRun()

	log.Println("⚠️ App terminating. Executing clean disconnect...")
	// gui.SendDisconnectCommandToDaemon()
	// gui.Engine.Disconnect()
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
		// 🛑 SECURITY: Do NOT fallback to direct connection.
		// If the daemon is unreachable, the gateway is likely down.
		log.Printf("❌ Daemon not found: %v. Please check the Claviger Service.", err)
		log.Println("Engine Disconnected", "The Claviger background service is unreachable. Please ensure it is running in Services.")
		return
	}

	defer conn.Close()
	log.Println("📡 Whispering CONNECT command to daemon...")

	payload := fmt.Sprintf("CONNECT|%s|%s", profile.ID, routing)
	if _, writeErr := conn.Write([]byte(payload)); writeErr != nil {
		log.Println("Communication Error", "Failed to send command to service.")
		return
	}

	// 2. Await Confirmation
	buf := make([]byte, 16)
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	n, readErr := conn.Read(buf)
	if readErr != nil {
		log.Println("Connection Timeout", "The service did not respond to the connect command.")
		return
	}

	response := strings.TrimSpace(string(buf[:n]))
	if response == "OK" {
		log.Println("✅ Connection delegated successfully.")
	} else {
		log.Println("Engine Rejected", fmt.Sprintf("The service returned: %s", response))
	}
}

func (gui *ClavigerGUI) SendDisconnectCommandToDaemon() {
	// Handle DISCONNECT
	// 🎯 1. ATTEMPT TO DELEGATE TO THE ROOT DAEMON
	conn, err := net.DialTimeout("tcp", "127.0.0.1:42899", 2*time.Second)
	if err == nil {
		defer conn.Close()
		log.Println("📡 Whispering DISCONNECT to root daemon...")

		if _, writeErr := conn.Write([]byte("DISCON")); writeErr == nil {
			// 🛑 CRITICAL FIX: Must wait for Ack here too, otherwise GUI might
			// refresh its state before the Daemon has actually finished tearing down!
			buf := make([]byte, 16)
			conn.SetReadDeadline(time.Now().Add(3 * time.Second))
			n, _ := conn.Read(buf)

			response := strings.TrimSpace(string(buf[:n]))
			if response == "OK" {
				log.Println("✅ Disconnect acknowledged by daemon.")
			} else {
				log.Printf("⚠️ Daemon replied with error: %s", response)
			}
		}
	}
}

// GetDaemonState pings the background service via TCP to get the true VPN state.
func (gui *ClavigerGUI) GetDaemonState() string {
	// 1. Quick dial to the daemon (1-second timeout so the UI doesn't freeze!)
	conn, err := net.DialTimeout("tcp", "127.0.0.1:42899", 1*time.Second)
	if err != nil {
		// If the daemon is dead or not installed, we assume it's offline.
		return vpn.StateDisconnected
	}
	defer conn.Close()

	// 2. Whisper the status command
	conn.Write([]byte("STATUS"))

	// 3. Read the exact string response from the Engine
	buf := make([]byte, 32)
	conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := conn.Read(buf)
	if err != nil {
		return vpn.StateDisconnected
	}

	response := strings.TrimSpace(string(buf[:n]))

	// Ensure the daemon didn't return garbage data
	switch response {
	case vpn.StateConnecting, vpn.StateVerifying, vpn.StateSecured, vpn.StateReconnecting:
		return response
	default:
		return vpn.StateDisconnected
	}
}

package main

import (
	"fmt"
	"log"
	"net"
	"os"

	"claviger-client/internal/cli"
	"claviger-client/internal/config"
	"claviger-client/internal/gui"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/dialog"
)

func main() {
	// 🎯 1. SINGLE INSTANCE LOCK
	// Attempt to bind to a specific local port. If it fails, Claviger is already running!
	listener, err := net.Listen("tcp", "127.0.0.1:42899")
	if err != nil {
		// Launch a tiny alert window to tell the user to check their system tray
		a := app.New()
		w := a.NewWindow("Claviger Network")
		d := dialog.NewInformation("Already Running", "Claviger is already running in your system tray!\nCheck the bottom right of your screen.", w)

		// When they click OK, close this duplicate instance
		d.SetOnClosed(func() { os.Exit(0) })
		d.Show()

		w.Resize(fyne.NewSize(350, 150))
		w.CenterOnScreen()
		w.ShowAndRun()
		return
	}
	defer listener.Close() // Keep the port locked until the app truly quits!

	// 2. Load the Secure Vault
	vault, vaultErr := config.Load()
	if vaultErr != nil {
		log.Fatalf("❌ Failed to load vault: %v", vaultErr)
	}

	// 3. HYBRID LAUNCHER: GUI vs CLI
	if len(os.Args) == 1 {
		gui.Run(vault)
		return
	}

	// 4. Command Router
	command := os.Args[1]
	switch command {
	case "generate":
		cli.HandleGenerate(vault)
	// ... (Leave the rest of your CLI switch cases exactly as they are!)
	case "approve":
		if len(os.Args) < 3 {
			log.Fatalf("❌ Usage: claviger approve <visa_token>")
		}
		cli.HandleApprove(vault, os.Args[2])
	case "list":
		cli.HandleList(vault)
	case "remove":
		if len(os.Args) < 3 {
			log.Fatalf("❌ Usage: claviger remove <profile_id>")
		}
		cli.HandleRemove(vault, os.Args[2])
	case "connect":
		cli.HandleConnect(vault, os.Args[2:])
	default:
		fmt.Printf("❌ Unknown command: %s\n", command)
		cli.PrintHelp()
	}
}

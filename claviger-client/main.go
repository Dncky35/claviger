package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"claviger-client/internal/api" // Ensure this matches your project's module path
	"claviger-client/internal/cli"
	"claviger-client/internal/config"
	"claviger-client/internal/gui"
	"claviger-client/internal/vpn"
)

// 1. Declare these at the PACKAGE level so every function can see them
var (
	vault  *config.ClientVault
	engine = vpn.NewEngine() // This is your persistent singleton engine
)

func main() {
	// 1. Check how the user launched the app FIRST
	isGUI := len(os.Args) == 1
	isDaemon := len(os.Args) > 1 && os.Args[1] == "daemon"
	isCLI := !isGUI && !isDaemon

	// 2. Route the logs based on the mode
	if isGUI || isDaemon {
		// Background tasks write to a log file silently
		logFile, err := os.OpenFile("/var/log/claviger-client-debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err == nil {
			log.SetOutput(logFile)
			defer logFile.Close()
		} else {
			log.SetOutput(os.Stdout)
		}
	} else {
		// CLI commands MUST print to the terminal so the user can see them!
		log.SetOutput(os.Stdout)
		log.SetFlags(0) // Optional: Removes the ugly date/time prefix for cleaner CLI output
	}

	log.Println("=====================================")
	log.Printf("🚀 Claviger Started. Mode check: GUI=%v, Daemon=%v, CLI=%v\n", isGUI, isDaemon, isCLI)

	var vaultErr error
	vault, vaultErr = config.Load()
	if vaultErr != nil {
		log.Fatalf("❌ Failed to load vault: %v", vaultErr)
	}

	wakeUpChan := make(chan bool)

	// 🎯 1. CREATE THE FIRE ALARM (CONTEXT)
	ctx, cancelFunc := context.WithCancel(context.Background())

	// Catch OS signals (like shutting down Linux or systemctl stop)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		<-sigChan
		log.Println("⚠️ OS Shutdown Signal received! Pulling the Fire Alarm...")
		cancelFunc() // Instantly stops the daemon safely
	}()

	// ---------------------------------------------------------
	// 🛑 SHORT-LIVED CLI ROUTE
	// ---------------------------------------------------------
	if isCLI {
		log.Printf("Executing CLI command: %v", os.Args[1:])

		command := os.Args[1]

		// Failsafe: if the user somehow bypassed the isDaemon check
		if command == "daemon" {
			log.Fatalf("❌ The daemon must be run by systemd (or without extra arguments)")
		}

		switch command {
		case "generate":
			cli.HandleGenerate(vault)
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
			// Pass the Context (ctx)
			cli.HandleConnect(vault, os.Args[2:], ctx)
		case "disconnect":
			cli.HandleDisconnect(vault)
		case "status":
			cli.HandleStatus(vault)
		case "autoconnect":
			if len(os.Args) < 3 {
				log.Fatalf("❌ Usage: claviger-client autoconnect <enable|disable>")
			}
			cli.HandleAutostart(vault, os.Args[2])
		case "global":
			if len(os.Args) < 3 {
				log.Fatalf("❌ Usage: claviger-client global <enable|disable>")
			}
			cli.HandleGlobalRouting(vault, os.Args[2])
		case "update":
			cli.HandleUpdate(vault)

		case "help":
			cli.PrintHelp()

		default:
			fmt.Printf("❌ Unknown command: %s\n", command)
			cli.PrintHelp()
		}

		// 🎯 EXIT HERE: Short-lived CLI commands do NOT start a server!
		return
	}

	// ---------------------------------------------------------
	// 🟢 LONG-RUNNING DAEMON & GUI ROUTE
	// ---------------------------------------------------------

	listenPort := "127.0.0.1:42899" // Default for Daemon
	if isGUI {
		listenPort = "127.0.0.1:42900" // GUI gets its own lock port!
	}

	log.Printf("Starting Background Listener on %s...", listenPort)

	// 🎯 MUST BE A GOROUTINE: Otherwise it blocks the GUI from starting!
	go api.StartListener(api.ListenerConfig{
		Ctx:        ctx,
		ListenPort: listenPort,
		IsGUI:      isGUI,
		WakeUpChan: wakeUpChan,
		CancelFunc: cancelFunc,
		Vault:      vault,
		Engine:     engine,
	})

	// ---------------------------------------------------------
	// 🤖 DAEMON-SPECIFIC LOGIC
	// ---------------------------------------------------------
	if isDaemon {
		log.Println("Starting Claviger Background Daemon...")

		// AUTO-CONNECT LOGIC
		if vault.AutoConnect && vault.ActiveProfileID != "" {
			if profile, exists := vault.Profiles[vault.ActiveProfileID]; exists {
				log.Printf("🔄 Auto-Connect enabled. Booting tunnel for %s...", profile.Name)

				go func() {
					err := engine.Connect(vault, profile, vault.UseGlobalRouting)
					if err != nil {
						log.Printf("❌ Auto-Connect failed: %v", err)
					} else {
						log.Println("✅ Auto-Connect successful!")
					}
				}()
			}
		}
	}

	// ---------------------------------------------------------
	// 🖥️ GUI-SPECIFIC LOGIC
	// ---------------------------------------------------------
	if isGUI {
		log.Println("Starting Claviger GUI...")
		gui.Run(vault, wakeUpChan) // This blocks until the user closes the GUI window

		log.Println("GUI Window Closed. Triggering clean shutdown...")
		cancelFunc() // Pull the fire alarm manually when GUI closes
	}

	// ---------------------------------------------------------
	// 🎯 FREEZE AND CLEANUP (Applies to Daemon & GUI)
	// ---------------------------------------------------------

	<-ctx.Done() // The ultimate anchor. Freezes here until cancelFunc is called!

	log.Println("🔔 Fire Alarm triggered (Context Canceled)! Executing clean disconnect...")
	engine.Disconnect()
	log.Println("👋 Claviger shut down gracefully. Network restored.")
	os.Exit(0)
}

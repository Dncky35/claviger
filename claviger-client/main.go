package main

import (
	"fmt"
	"log"
	"net"
	"os"

	"claviger-client/internal/cli"
	"claviger-client/internal/config"
	"claviger-client/internal/gui"
)

func main() {
	isGUI := len(os.Args) == 1

	// 🎯 A channel to pass the "Wake Up" signal from the network to the Fyne UI
	wakeUpChan := make(chan bool)

	// 1. SMART SINGLE INSTANCE LOCK & WAKEUP
	listener, err := net.Listen("tcp", "127.0.0.1:42899")
	if err != nil {
		// Port is taken! The app is already running.
		if isGUI {
			// We are Instance B. Connect to Instance A and whisper "WAKEUP"
			conn, dialErr := net.Dial("tcp", "127.0.0.1:42899")
			if dialErr == nil {
				conn.Write([]byte("WAKEUP"))
				conn.Close()
			}
			os.Exit(0) // Quit silently!
		}
		log.Fatalf("❌ Claviger is already running in the background.")
	}
	defer listener.Close()

	// 2. We are Instance A! Start listening for whispers in the background.
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				continue
			}
			buf := make([]byte, 6)
			conn.Read(buf)
			if string(buf) == "WAKEUP" {
				wakeUpChan <- true // Tell the GUI to show itself!
			}
			conn.Close()
		}
	}()

	// 3. Load the Secure Vault
	vault, vaultErr := config.Load()
	if vaultErr != nil {
		log.Fatalf("❌ Failed to load vault: %v", vaultErr)
	}

	// 4. HYBRID LAUNCHER
	if isGUI {
		// Pass the channel into the GUI so it can react
		gui.Run(vault, wakeUpChan)
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

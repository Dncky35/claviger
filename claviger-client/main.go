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
	logFile, err := os.OpenFile("claviger-debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err == nil {
		log.SetOutput(logFile)
		defer logFile.Close()
	}
	log.Println("=====================================")
	log.Println("🚀 Claviger Started. isGUI check executing...")

	isGUI := len(os.Args) == 1
	wakeUpChan := make(chan bool)

	// 1. SMART SINGLE INSTANCE LOCK & WAKEUP
	listener, err := net.Listen("tcp", "127.0.0.1:42899")
	if err != nil {
		log.Printf("⚠️ Port 42899 taken or blocked: %v", err)
		if isGUI {
			conn, dialErr := net.Dial("tcp", "127.0.0.1:42899")
			if dialErr == nil {
				conn.Write([]byte("WAKEUP"))
				conn.Close()
				log.Println("Woke up existing instance. Exiting.")
				os.Exit(0) // Valid wakeup!
			}
			// 🎯 THE FIX: If Dial fails, it means the port is blocked by Windows,
			// NOT by another Claviger app. We should continue loading!
			log.Printf("Could not wake up app (Dial failed: %v). Continuing anyway.", dialErr)
		} else {
			log.Fatalf("❌ Claviger is already running or port is blocked.")
		}
	} else {
		defer listener.Close()
		// We are Instance A! Start listening for whispers.
		go func() {
			for {
				conn, err := listener.Accept()
				if err != nil {
					continue
				}
				buf := make([]byte, 6)
				conn.Read(buf)
				if string(buf) == "WAKEUP" {
					wakeUpChan <- true
				}
				conn.Close()
			}
		}()
	}

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

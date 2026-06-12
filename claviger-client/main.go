package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"strings"

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
	logFile, err := os.OpenFile("claviger-client-debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)

	if err == nil {
		log.SetOutput(logFile)
		defer logFile.Close()
	}
	log.Println("=====================================")
	log.Println("🚀 Claviger Started. isGUI check executing...")

	var vaultErr error
	vault, vaultErr = config.Load()
	if vaultErr != nil {
		log.Fatalf("❌ Failed to load vault: %v", vaultErr)
	}

	isGUI := len(os.Args) == 1
	wakeUpChan := make(chan bool)
	disconnectChan := make(chan bool) // 👈 Used to gracefully kill the VPN from terminal

	listenPort := "127.0.0.1:42899" // Default for CLI/Daemon
	if isGUI {
		listenPort = "127.0.0.1:42900" // GUI gets its own lock port!
	}

	// 1. SMART SINGLE INSTANCE LOCK & COMMAND SERVER
	listener, err := net.Listen("tcp", listenPort)
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
			// For CLI commands, failing to listen just means the background process is already running.
			// The commands below (like 'status' or 'disconnect') will dial in and work perfectly!
		}
	} else {
		defer listener.Close()

		// 2. We are Instance A! Start listening for IPC whispers (GUI Wakeup or Terminal Shutdown).
		go func() {
			for {
				conn, err := listener.Accept()
				if err != nil {
					continue
				}

				buf := make([]byte, 128)
				n, _ := conn.Read(buf)
				commandReceived := string(buf[:n])

				// Parse commands that have payloads (like CONNECT|profile_123|global)
				parts := strings.Split(commandReceived, "|")
				baseCommand := parts[0]

				switch baseCommand {
				case "WAKEUP":
					wakeUpChan <- true
					conn.Close()

				case "DISCON":
					conn.Write([]byte("Engine disconnecting..."))
					conn.Close()
					disconnectChan <- true

				case "STATUS":
					conn.Write([]byte("ONLINE"))
					conn.Close()

				// 🎯 THE NEW CONNECT HANDLER FOR LINUX
				case "CONNECT":
					if len(parts) >= 3 {
						targetID := parts[1]
						routeMode := parts[2]
						useGlobal := (routeMode == "global")

						if profile, exists := vault.Profiles[targetID]; exists {
							log.Printf("Root Daemon received CONNECT command for profile: %s", targetID)

							// 1. ACKNOWLEDGE FIRST
							conn.Write([]byte("OK"))
							conn.Close() // Now it is safe to close!

							// 2. RUN ENGINE
							go func() {
								// IMPORTANT: Use your existing engine instance (see below)
								err := engine.Connect(vault, profile, useGlobal)
								if err != nil {
									log.Printf("Daemon Connect Error: %v", err)
								}
							}()
						} else {
							conn.Write([]byte("ERROR: Profile not found"))
							conn.Close()
						}
					}

				default:
					conn.Close()
				}
			}
		}()
	}

	// 4. HYBRID LAUNCHER
	if isGUI {
		// Pass the channel into the GUI so it can react
		gui.Run(vault, wakeUpChan)
		return
	}

	// 5. Command Router
	command := os.Args[1]
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
		// 🎯 Pass the disconnectChan so it listens for the remote shutdown signal!
		cli.HandleConnect(vault, os.Args[2:], disconnectChan)
	case "disconnect":
		cli.HandleDisconnect(vault)
	case "status":
		cli.HandleStatus(vault)
	case "daemon":
		log.Println("Starting Claviger Background Daemon...")
		// Here is where you call the function that starts your VPN,
		// configures UFW, and stays open forever.
		// Example: engine.StartVPNController(vault)

		// To prevent the program from exiting immediately, you block it:
		select {} // This keeps the Go routine alive forever
	default:
		fmt.Printf("❌ Unknown command: %s\n", command)
		cli.PrintHelp()
	}
}

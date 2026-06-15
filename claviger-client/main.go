package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"strings"

	"claviger-client/internal/auth"
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
	} else {
		log.SetOutput(os.Stdout)
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
			conn, dialErr := net.Dial("tcp", "127.0.0.1:42900")
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

		// 2. We are Instance A! Start listening for IPC whispers.
		go func() {
			for {
				conn, err := listener.Accept()
				if err != nil {
					continue
				}

				// 🛑 CRITICAL FIX: Handle the connection in a background goroutine!
				go func(c net.Conn) {
					defer c.Close() // Ensures connection always closes when done

					buf := make([]byte, 256)
					n, err := c.Read(buf)
					if err != nil || n == 0 {
						return // Drop empty or broken connections gracefully
					}

					commandReceived := string(buf[:n])
					parts := strings.Split(commandReceived, "|")
					baseCommand := parts[0]

					switch baseCommand {
					case "WAKEUP":
						wakeUpChan <- true

						// Inside main.go -> TCP Listener goroutine -> switch baseCommand:

					case "APPROVE":
						log.Println("DEBUG: Daemon entered APPROVE case") // 👈 IS THIS LOGGING?
						if len(parts) >= 2 {
							tokenString := parts[1]
							log.Printf("Root Daemon received APPROVE command.")

							// Check if vault is Nil
							if vault == nil {
								log.Println("ERROR: Daemon vault is nil!")
								c.Write([]byte("ER"))
								return
							}

							// 1. Decode the token
							approval, err := auth.DecodeApprovalToken(tokenString)
							if err != nil {
								log.Printf("Daemon failed to decode token: %v", err)
								c.Write([]byte("ER"))
								return // exit this case
							}

							// 2. Update the Daemon's copy of the Vault
							if vault.ActiveProfileID != "" {
								if profile, exists := vault.Profiles[vault.ActiveProfileID]; exists {
									profile.AssignedIP = approval.AssignedIP
									profile.ServerKey = approval.ServerPubKey
									profile.ServerEndpoint = approval.ServerEndpoint
									profile.DNS = approval.DNS
									profile.BaseSubnet = approval.BaseSubnet
									profile.Status = "active"
									profile.HubPort = approval.HubPort

									serverIP := strings.Split(approval.ServerEndpoint, ":")[0]
									profile.Name = fmt.Sprintf("Claviger Hub (%s)", serverIP)

									// 3. Save as ROOT to /etc/claviger/vault.json
									if err := config.Save(vault); err == nil {
										log.Println("✅ Root Daemon successfully saved updated Vault.")
										c.Write([]byte("OK")) // Tell the GUI it worked!
									} else {
										log.Printf("❌ Root Daemon failed to save: %v", err)
										c.Write([]byte("ER"))
									}
								} else {
									c.Write([]byte("ER"))
								}
							} else {
								c.Write([]byte("ER"))
							}
						}

					case "DISCON":
						disconnectChan <- true
						c.Write([]byte("OK"))

					case "STATUS":
						// Get the real-time state from your VPN engine
						// (Assuming your engine has a GetState() method returning "Connected", "Disconnected", etc.)
						currentState := engine.GetState()

						// Fallback if your engine state string is empty
						if currentState == "" {
							currentState = "DEAMON ONLINE" // At least we know the daemon is running
						}

						c.Write([]byte(currentState))

					// 🎯 THE NEW CONNECT HANDLER FOR LINUX
					case "CONNECT":
						if len(parts) >= 3 {
							targetID := parts[1]
							routeMode := parts[2]
							useGlobal := (routeMode == "global")

							log.Printf("Target Id: %s, Route Mode: %s", targetID, routeMode)

							if profile, exists := vault.Profiles[targetID]; exists {
								log.Printf("Root Daemon received CONNECT command for profile: %s", targetID)

								// 1. ACKNOWLEDGE FIRST so the GUI can continue
								c.Write([]byte("OK"))

								// 2. RUN ENGINE
								go func() {
									err := engine.Connect(vault, profile, useGlobal)
									if err != nil {
										log.Printf("Daemon Connect Error: %v", err)
									}
								}()
							} else {
								c.Write([]byte("ER")) // Error: Profile not found
							}
						}
					}
				}(conn) // Pass the connection into the goroutine
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
	// 🎯 NEW: Standalone Autostart Toggle
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

	case "daemon":

		log.Println("Starting Claviger Background Daemon...")

		// 🎯 AUTO-CONNECT LOGIC
		if vault.AutoConnect && vault.ActiveProfileID != "" {
			if profile, exists := vault.Profiles[vault.ActiveProfileID]; exists {
				log.Printf("🔄 Auto-Connect enabled. Booting tunnel for %s...", profile.Name)

				// Run the engine in the background
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

		// Keep the daemon alive and listening for future GUI commands
		select {}

	case "uninstall":
		cli.HandleUninstall()

	default:
		fmt.Printf("❌ Unknown command: %s\n", command)
		cli.PrintHelp()
	}
}

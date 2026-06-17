package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

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
	// 1. Check how the user launched the app FIRST
	isGUI := len(os.Args) == 1
	isDaemon := len(os.Args) > 1 && os.Args[1] == "daemon"

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
	log.Println("🚀 Claviger Started. isGUI check executing...")

	var vaultErr error
	vault, vaultErr = config.Load()
	if vaultErr != nil {
		log.Fatalf("❌ Failed to load vault: %v", vaultErr)
	}

	// isGUI := len(os.Args) == 1
	wakeUpChan := make(chan bool)

	// 🎯 1. CREATE THE FIRE ALARM (CONTEXT)
	// We replace disconnectChan with a Context. This is our master lifecycle switch.
	ctx, cancelFunc := context.WithCancel(context.Background())

	// Catch OS signals (like shutting down Linux or systemctl stop) and pull the fire alarm!
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		<-sigChan
		log.Println("⚠️ OS Shutdown Signal received! Pulling the Fire Alarm...")
		cancelFunc() // Instantly stops the daemon safely
	}()

	listenPort := "127.0.0.1:42899" // Default for CLI/Daemon
	if isGUI {
		listenPort = "127.0.0.1:42900" // GUI gets its own lock port!
	}

	// 2. SMART SINGLE INSTANCE LOCK & COMMAND SERVER
	listener, err := net.Listen("tcp", listenPort)
	if err != nil {
		log.Printf("⚠️ Port %s taken or blocked: %v", listenPort, err)
		if isGUI {
			conn, dialErr := net.Dial("tcp", "127.0.0.1:42900")
			if dialErr == nil {
				conn.Write([]byte("WAKEUP"))
				conn.Close()
				log.Println("Woke up existing instance. Exiting.")
				os.Exit(0) // Valid wakeup!
			}
			log.Printf("Could not wake up app (Dial failed: %v). Continuing anyway.", dialErr)
		}
	} else {
		defer listener.Close()

		// We are Instance A! Start listening for IPC whispers.
		go func() {
			for {
				conn, err := listener.Accept()
				if err != nil {
					continue
				}

				// Handle the connection in a background goroutine!
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

					case "APPROVE":
						log.Println("DEBUG: Daemon entered APPROVE case")
						if len(parts) >= 2 {
							tokenString := parts[1]
							log.Printf("Root Daemon received APPROVE command.")

							if vault == nil {
								log.Println("ERROR: Daemon vault is nil!")
								c.Write([]byte("ER"))
								return
							}

							approval, err := auth.DecodeApprovalToken(tokenString)
							if err != nil {
								log.Printf("Daemon failed to decode token: %v", err)
								c.Write([]byte("ER"))
								return
							}

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

									if err := config.Save(vault); err == nil {
										log.Println("✅ Root Daemon successfully saved updated Vault.")
										c.Write([]byte("OK"))
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

					// 🎯 2. PULL THE ALARM ON DISCONNECT
					case "DISCON":
						log.Println("🛑 Remote Disconnect command received via TCP.")
						cancelFunc() // Pulls the Fire Alarm instantly!
						c.Write([]byte("OK"))

					case "STATUS":
						currentState := engine.GetState()
						if currentState == "" {
							currentState = "ONLINE" // At least we know the daemon is running
						}
						c.Write([]byte(currentState))

					case "CONNECT":
						if len(parts) >= 3 {
							targetID := parts[1]
							routeMode := parts[2]
							useGlobal := (routeMode == "global")

							log.Printf("Target Id: %s, Route Mode: %s", targetID, routeMode)

							if profile, exists := vault.Profiles[targetID]; exists {
								log.Printf("Root Daemon received CONNECT command for profile: %s", targetID)

								c.Write([]byte("OK"))

								go func() {
									err := engine.Connect(vault, profile, useGlobal)
									if err != nil {
										log.Printf("Daemon Connect Error: %v", err)
									}
								}()
							} else {
								c.Write([]byte("ER"))
							}
						}
					}
				}(conn) // Pass the connection into the goroutine
			}
		}()
	}

	// 4. HYBRID LAUNCHER
	if isGUI {
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
		// 🎯 3. Pass the Context (ctx) instead of the old channel
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

	case "daemon":
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

		// 🎯 4. FREEZE AND WAIT FOR THE ALARM
		// This replaces your old "select {}" which blocked forever and deadlocked.
		// The daemon sits right here happily handling traffic until `cancelFunc()` is called.
		<-ctx.Done()

		// 🎯 5. EXECUTE THE CLEANUP
		log.Println("🔔 Fire Alarm triggered (Context Canceled)! Executing clean disconnect...")
		engine.Disconnect()
		log.Println("👋 Claviger Daemon shut down gracefully. Network restored.")
		os.Exit(0)

	case "uninstall":
		cli.HandleUninstall()

	default:
		fmt.Printf("❌ Unknown command: %s\n", command)
		cli.PrintHelp()
	}
}

package api

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"strings"

	"claviger-client/internal/auth"
	"claviger-client/internal/config"
	// ❌ REMOVED: "claviger-client/internal/vpn"
)

// 🎯 1. Define the interface so the API doesn't need to import the VPN package
type VPNEngine interface {
	GetState() string
	Connect(vault *config.ClientVault, profile *config.ServerProfile, useGlobal bool) error
}

// ListenerConfig holds the dependencies needed by the TCP listener
type ListenerConfig struct {
	Ctx        context.Context
	ListenPort string
	IsGUI      bool
	WakeUpChan chan<- bool
	CancelFunc context.CancelFunc
	Vault      *config.ClientVault
	Engine     VPNEngine // 🎯 2. Use the interface here instead of *vpn.Engine
}

// StartListener handles the single-instance lock and starts the IPC command server
func StartListener(cfg ListenerConfig) {
	listener, err := net.Listen("tcp", cfg.ListenPort)
	if err != nil {
		log.Printf("⚠️ Port %s taken or blocked: %v", cfg.ListenPort, err)

		if cfg.IsGUI {
			// Single Instance Lock for GUI
			conn, dialErr := net.Dial("tcp", cfg.ListenPort)
			if dialErr == nil {
				conn.Write([]byte("WAKEUP"))
				conn.Close()
				log.Println("Woke up existing GUI instance. Exiting.")
				os.Exit(0)
			}
			log.Printf("Could not wake up app (Dial failed: %v). Continuing anyway.", dialErr)
			return // GUI can return and try to boot anyway if dial failed
		}

		// 🔴 FIX: If it is the DAEMON, it MUST crash here. Two daemons cannot share one machine!
		log.Fatalf("❌ FATAL: Claviger Daemon is already running (Port %s is locked).", cfg.ListenPort)
	}

	// 🔴 FIX: Start a dedicated goroutine just to listen for the Fire Alarm
	go func() {
		<-cfg.Ctx.Done() // Waits here until cancelFunc() is called in main.go
		log.Println("🛑 Closing IPC Listener due to Fire Alarm...")
		listener.Close() // This forces listener.Accept() to unblock and return an error!
	}()

	// The actual listening loop
	go func() {
		// No need for defer listener.Close() here anymore, the Fire Alarm handles it!
		for {
			conn, err := listener.Accept()
			if err != nil {
				// If the fire alarm was pulled, Accept() returns an error. We exit the loop cleanly.
				select {
				case <-cfg.Ctx.Done():
					return
				default:
					log.Printf("❌ Listener accept error: %v", err)
					continue
				}
			}
			go handleConnection(conn, cfg)
		}
	}()
}

func handleConnection(c net.Conn, cfg ListenerConfig) {
	defer c.Close()

	buf := make([]byte, 256)
	n, err := c.Read(buf)
	if err != nil || n == 0 {
		return
	}

	// 🛡️ SAFETY 1: Trim any invisible newline characters from the TCP payload
	commandReceived := strings.TrimSpace(string(buf[:n]))
	parts := strings.Split(commandReceived, "|")
	baseCommand := parts[0]

	switch baseCommand {
	case "WAKEUP":
		// 🛡️ SAFETY 2: Non-blocking channel send to prevent deadlocks
		select {
		case cfg.WakeUpChan <- true:
			log.Println("WAKEUP signal sent to main loop.")
		default:
			log.Println("WAKEUP signal dropped (loop is busy or already awake).")
		}

	case "APPROVE":
		log.Println("DEBUG: Daemon entered APPROVE case")
		if len(parts) >= 2 {
			tokenString := parts[1]
			log.Printf("Root Daemon received APPROVE command.")

			if cfg.Vault == nil {
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

			if cfg.Vault.ActiveProfileID != "" {
				if profile, exists := cfg.Vault.Profiles[cfg.Vault.ActiveProfileID]; exists {
					profile.AssignedIP = approval.AssignedIP
					profile.ServerKey = approval.ServerPubKey
					profile.ServerEndpoint = approval.ServerEndpoint
					profile.DNS = approval.DNS
					profile.BaseSubnet = approval.BaseSubnet
					profile.Status = "active"
					profile.HubPort = approval.HubPort

					serverIP := strings.Split(approval.ServerEndpoint, ":")[0]
					profile.Name = fmt.Sprintf("Claviger Hub (%s)", serverIP)

					if err := config.Save(cfg.Vault); err == nil {
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

	case "DISCON":
		vpnState := cfg.Engine.GetState()
		if vpnState == "Disconnected" {
			c.Write([]byte("ER"))
			return
		}

		log.Println("🛑 Remote Disconnect command received via TCP.")
		if cfg.CancelFunc != nil {
			cfg.CancelFunc() // This cleanly pulls the fire alarm!
		}
		c.Write([]byte("OK"))

	case "STATUS":
		currentState := cfg.Engine.GetState()
		if currentState == "" {
			currentState = "ONLINE"
		}
		c.Write([]byte(currentState))

	case "CONNECT":

		vpnState := cfg.Engine.GetState()
		if vpnState != "Disconnected" {
			c.Write([]byte("ER"))
			return
		}

		if len(parts) >= 3 {
			targetID := parts[1]
			routeMode := parts[2]
			useGlobal := (routeMode == "global")

			// 🛡️ SAFETY 3: Load fresh vault safely
			freshVault, err := config.Load()
			if err == nil {
				*cfg.Vault = *freshVault
			}

			log.Printf("Target Id: %s, Route Mode: %s", targetID, routeMode)

			if profile, exists := cfg.Vault.Profiles[targetID]; exists {
				log.Printf("Root Daemon received CONNECT command for profile: %s", targetID)

				c.Write([]byte("OK"))

				// Spin off the connection process so we don't block the TCP listener
				go func() {
					err := cfg.Engine.Connect(cfg.Vault, profile, useGlobal)
					if err != nil {
						log.Printf("Daemon Connect Error: %v", err)
					}
				}()
			} else {
				c.Write([]byte("ER"))
			}
		}
	}
}

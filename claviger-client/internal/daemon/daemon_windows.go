//go:build windows

package daemon

import (
	"context"
	"log"
	"time"

	"claviger-client/internal/config"
	"claviger-client/internal/vpn"

	"golang.org/x/sys/windows/svc"
)

// 1. Define our Windows Service Struct
type clavigerService struct {
	vault  *config.ClientVault
	engine *vpn.Engine
}

// 2. Implement the Execute interface required by Windows SCM
func (m *clavigerService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown

	// Tell Windows we are currently starting up
	changes <- svc.Status{State: svc.StartPending}
	log.Println("🪟 Windows SCM: Claviger Service Starting...")

	// Create a context specifically tied to the lifespan of this Windows Service
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // Ensure no context leaks

	// -----------------------------------------------------
	// KICK OFF BACKGROUND ENGINE LOGIC HERE
	// -----------------------------------------------------
	if m.vault.AutoConnect && m.vault.ActiveProfileID != "" {
		if profile, exists := m.vault.Profiles[m.vault.ActiveProfileID]; exists {
			log.Printf("🔄 Auto-Connect enabled. Queuing tunnel boot for %s...", profile.Name)

			go func() {
				retryDelay := 2 * time.Second
				maxDelay := 30 * time.Second

				for {
					// 1. Check if SCM requested shutdown before trying
					select {
					case <-ctx.Done():
						log.Println("🛑 Auto-Connect aborted due to Windows service shutdown.")
						return
					default:
						// Proceed to connect
					}

					// 2. Attempt the connection
					err := m.engine.Connect(m.vault, profile, m.vault.UseGlobalRouting)
					if err == nil {
						log.Println("✅ Auto-Connect successful! Network established.")
						return
					}

					// 3. Handle Failure and Backoff
					log.Printf("⏳ Network not ready (%v). Retrying in %v...", err, retryDelay)

					// Wait for delay, but abort immediately if Windows shuts down
					select {
					case <-ctx.Done():
						log.Println("🛑 Auto-Connect aborted during backoff sleep.")
						return
					case <-time.After(retryDelay):
						retryDelay *= 2
						if retryDelay > maxDelay {
							retryDelay = maxDelay
						}
					}
				}
			}()
		}
	}

	// Tell Windows we are fully running and ready to accept stop commands
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

	// 3. Block and listen for commands from Windows (Services.msc)
loop:
	for c := range r {
		switch c.Cmd {
		case svc.Interrogate:
			changes <- c.CurrentStatus
		case svc.Stop, svc.Shutdown:
			log.Println("🛑 Windows SCM requested shutdown. Tearing down Claviger Engine...")

			// 🚨 CRITICAL: Cancel the context first!
			// This instantly kills the Auto-Connect backoff loop if it is currently sleeping.
			cancel()

			// Synchronous Disconnect - DO NOT use a 'go' routine here!
			// We must block Windows from killing the process until the network stack is clean.
			if m.engine != nil {
				err := m.engine.Disconnect()
				if err != nil {
					log.Printf("⚠️ Engine teardown encountered an error: %v", err)
				} else {
					log.Println("✅ Network adapters and firewall rules cleanly restored.")
				}
			}

			break loop
		default:
			log.Printf("⚠️ Unexpected control request from SCM #%d", c)
		}
	}

	// Tell Windows we are shutting down
	changes <- svc.Status{State: svc.StopPending}
	return
}

func RunDaemon(ctx context.Context, vault *config.ClientVault, engine *vpn.Engine) {
	// Detect if a user ran this in terminal, or if Windows Boot Manager ran it
	inService, err := svc.IsWindowsService()
	if err != nil {
		log.Fatalf("Failed to determine session type: %v", err)
	}

	if inService {
		log.Println("🛡️ Running natively under Windows Service Control Manager (SYSTEM).")
		// Hand control over to Windows SCM
		err = svc.Run("ClavigerService", &clavigerService{vault: vault, engine: engine})
		if err != nil {
			log.Fatalf("Claviger Windows Service crashed: %v", err)
		}
	} else {
		log.Println("⚠️ Running Windows Daemon in interactive terminal mode (Not SCM).")
		// Fallback for when you test it manually in the CLI
		<-ctx.Done() // Wait patiently until the context is cancelled
		log.Println("👋 Interactive Daemon shutting down...")
	}
}

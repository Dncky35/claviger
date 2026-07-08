//go:build windows

package daemon

import (
	"claviger-client/internal/config"
	"claviger-client/internal/vpn"
	"log"

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

	// -----------------------------------------------------
	// KICK OFF BACKGROUND ENGINE LOGIC HERE
	// -----------------------------------------------------
	if m.vault.AutoConnect && m.vault.ActiveProfileID != "" {
		if profile, exists := m.vault.Profiles[m.vault.ActiveProfileID]; exists {
			log.Printf("🔄 Auto-Connect enabled. Booting tunnel for %s...", profile.Name)
			go m.engine.Connect(m.vault, profile, m.vault.UseGlobalRouting)
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

			log.Println("🛑 Windows SCM requested shutdown. Tearing down Claviger Engine...")

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

// 4. The entrypoint called by main.go
func RunDaemon(vault *config.ClientVault, engine *vpn.Engine) {
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
		select {}
	}
}

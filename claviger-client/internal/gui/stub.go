//go:build headless

package gui

import (
	"claviger-client/internal/config"
	"log"
)

// Run acts as a dummy replacement for the GUI when compiled for headless servers
func Run(vault *config.ClientVault, wakeUpChan chan bool) {
	log.Fatalf("❌ This binary was compiled as Headless-Only. Please run with CLI arguments (e.g., './claviger connect <server>').")
}

package main

import (
	"fmt"
	"log"
	"os"

	"claviger-client/internal/cli"
	"claviger-client/internal/config"
	"claviger-client/internal/gui" // 🎯 Import our new GUI package
)

func main() {
	// 1. Load the Secure Vault
	vault, err := config.Load()
	if err != nil {
		log.Fatalf("❌ Failed to load vault: %v", err)
	}

	// 🎯 2. HYBRID LAUNCHER: If no commands are passed, launch the GUI!
	if len(os.Args) == 1 {
		gui.Run(vault)
		return // Exit when the GUI is closed
	}

	// 3. Command Router (For Headless CLI)
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
		cli.HandleConnect(vault, os.Args[2:])
	default:
		fmt.Printf("❌ Unknown command: %s\n", command)
		cli.PrintHelp()
	}
}

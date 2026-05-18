package main

import (
	"fmt"
	"log"
	"os"

	"claviger-client/internal/cli"
	"claviger-client/internal/config"
)

func main() {
	// 1. Load the Secure Vault
	vault, err := config.Load()
	if err != nil {
		log.Fatalf("❌ Failed to load vault: %v", err)
	}

	// 2. Parse basic arguments
	if len(os.Args) < 2 {
		cli.PrintHelp()
		os.Exit(1)
	}

	command := os.Args[1]

	// 3. Command Router
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
		// Pass everything after "connect" to the handler so it can parse IDs and flags
		cli.HandleConnect(vault, os.Args[2:])
	default:
		fmt.Printf("❌ Unknown command: %s\n", command)
		cli.PrintHelp()
	}
}

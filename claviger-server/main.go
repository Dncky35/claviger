package main

import (
	"fmt"
	"os"

	"claviger-server/cmd" // Imports your command package
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		return
	}

	command := os.Args[1]
	args := os.Args[2:]

	switch command {
	case "setup":
		cmd.RunSetup(args) // Calls the logic from cmd/setup.go
	case "reset":
		cmd.RunReset() // Calls the logic from cmd/reset.go
	case "start":
		fmt.Println("Starting Claviger daemon...")
		// cmd.RunStart()
	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
	}
}

func printUsage() {
	fmt.Println("=== Claviger Edge Node ===")
	fmt.Println("Usage:")
	fmt.Println("  claviger setup  - Initialize the server, create DB, and authenticate SaaS")
	fmt.Println("  claviger start  - Run the VPN daemon and Local Admin Hub")
}

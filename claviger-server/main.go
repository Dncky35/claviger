package main

import (
	"fmt"
	"os"

	"claviger-server/cmd"
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
	case "uninstall":
		cmd.RunUninstall() // Calls the logic from cmd/uninstall.go
	case "start":
		cmd.RunStart() // Calls the logic from cmd/start.go
	case "register":
		cmd.RunRegisterClient() // Calls the logic from cmd/register.go

	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
	}
}

func printUsage() {
	fmt.Println("=== Claviger Edge Node ===")
	fmt.Println("Usage:")
	fmt.Println("  claviger-server  					- Provision the server, DB, and VPN keys")
	fmt.Println("  claviger-server uninstall            - Uninstall the VPN daemon and Local Web Hub")
	fmt.Println("  claviger-server reset                - Safely wipe the local configuration")
	fmt.Println("  claviger-server start                - Boot the VPN daemon and Local Web Hub")
	fmt.Println("  claviger-server register             - Register a new client")
}

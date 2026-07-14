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
		cmd.RunSetup(args)
	case "reset":
		cmd.RunReset()
	case "uninstall":
		cmd.RunUninstall()
	case "start":
		cmd.RunStart()
	case "register":
		cmd.RunRegisterClient()
	case "recovery-key":
		cmd.ShowRecoveryKey()
	case "list":
		cmd.RunGetList(args) // Updated: Pass args to filter by role
	case "revoke":
		if len(args) < 1 {
			fmt.Println("❌ Usage: claviger-server revoke [client_id]")
			return
		}
		cmd.RunRevokeClient(args[0]) // New: Passes the ID to the terminator logic

	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
	}
}

func printUsage() {
	fmt.Println("=== Claviger Zero Trust Gateway - Server CLI ===")
	fmt.Println("Usage:")
	fmt.Println("  claviger-server setup                - Provision the server, DB, and VPN keys")
	fmt.Println("  claviger-server start                - Boot the VPN daemon and Local Web Hub")
	fmt.Println("  claviger-server register             - Register a new client")
	fmt.Println("  claviger-server list                 - List all registered VPN clients")
	fmt.Println("  claviger-server list [role]          - List clients filtered by role (e.g., admin)")
	fmt.Println("  claviger-server revoke [id]          - Revoke a client and drop their active session")
	fmt.Println("  claviger-server reset                - Safely wipe the local configuration")
	fmt.Println("  claviger-server uninstall            - Uninstall the VPN daemon and Local Web Hub")
}

package main

import (
	"fmt"
	"os"

	"claviger-server/cmd"
	"claviger-server/storage"
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
	case "restore":
		if len(args) < 1 {
			fmt.Println("❌ Usage: claviger-server restore [backup_file]")
			return
		}
		cmd.RunRestore(args[0]) // New: Passes the path to the backup file

	case "revoke":
		if len(args) < 1 {
			fmt.Println("❌ Usage: claviger-server revoke [client_id]")
			return
		}
		cmd.RunRevokeClient(args[0]) // New: Passes the ID to the terminator logic

	case "register-node":
		if len(args) < 2 {
			fmt.Println("❌ Usage: claviger-server register-node [master_ip] [sub_server_vpn_ip]")
			fmt.Println("   Example: claviger-server register-node 10.8.0.1 10.8.0.5")
			return
		}

		// Initialize local SQLite connection
		db := storage.InitDB() // Ensure this matches your storage DB initializer
		defer db.Close()

		masterIP := args[0]
		subServerIP := args[1] // We now pass the IP, not the ID

		if err := cmd.RunCreateNode(db, masterIP, subServerIP); err != nil {
			fmt.Printf("❌ Node registration failed: %v\n", err)
			os.Exit(1)
		}

	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
	}
}

func printUsage() {
	fmt.Println("Claviger Management CLI")
	fmt.Println("Usage:")
	fmt.Println("  claviger-server setup                        Run initial setup")
	fmt.Println("  claviger-server start                        Start server daemon")
	fmt.Println("  claviger-server register                     Register new VPN client")
	fmt.Println("  claviger-server register-node <ip> <id>      Register this server as a Node on Master")
	fmt.Println("  claviger-server list [role]                  List registered clients")
	fmt.Println("  claviger-server revoke <client_id>           Revoke a client access")
	fmt.Println("  claviger-server restore <file>               Restore from backup")
	fmt.Println("  claviger-server recovery-key                 Display system recovery key")
	fmt.Println("  claviger-server reset                        Reset all configurations")
	fmt.Println("  claviger-server uninstall                    Uninstall Claviger")
}

package cmd

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"claviger-server/internal/auth"
	"claviger-server/storage"
)

func RunRegisterClient() {
	db := storage.InitDB()
	defer db.Close()

	// --- THE FIX: Ensure Setup Was Completed ---
	if storage.GetConfig(db, "node_id") == "" {
		log.Fatal("❌ Node is not configured! Please run 'sudo claviger-server setup' first.")
	}

	reader := bufio.NewReader(os.Stdin)

	fmt.Println("==================================================")
	fmt.Println("           CLAVIGER DEVICE ENROLLMENT             ")
	fmt.Println("==================================================")

	// 1. Get the Passport Token
	fmt.Print("\nPaste the Client's Request Token (Passport): ")
	tokenInput, _ := reader.ReadString('\n')
	tokenInput = strings.TrimSpace(tokenInput)

	if tokenInput == "" {
		log.Fatal("❌ Token cannot be empty.")
	}

	// 2. Decode the Token
	connReq, err := auth.DecodeConnectionRequest(tokenInput)
	if err != nil {
		log.Fatalf("❌ Invalid or corrupted Connection Request token: %v", err)
	}

	fmt.Printf("\n[i] Device Identified:\n    - Hostname:  %s\n    - Platform:  %s\n    - Device ID: %s\n\n",
		connReq.Hostname, connReq.Hostname, connReq.DeviceID)

	// 3. Fetch Available Roles from the Database
	rows, err := db.Query("SELECT id, name FROM roles")
	if err != nil {
		log.Fatalf("❌ Failed to query roles: %v", err)
	}
	defer rows.Close()

	type Role struct {
		ID   string
		Name string
	}
	var roles []Role

	for rows.Next() {
		var r Role
		if err := rows.Scan(&r.ID, &r.Name); err != nil {
			log.Fatalf("❌ Failed to read role row: %v", err)
		}
		roles = append(roles, r)
	}

	if len(roles) == 0 {
		log.Fatal("❌ No roles found in the database. Setup may be corrupted.")
	}

	// 4. Prompt the user to select a role
	fmt.Println("Available Network Roles:")
	for i, r := range roles {
		fmt.Printf("  [%d] %s\n", i+1, r.Name)
	}

	fmt.Print("\nSelect a role by number: ")
	roleSelection, _ := reader.ReadString('\n')
	roleSelection = strings.TrimSpace(roleSelection)

	roleIdx, err := strconv.Atoi(roleSelection)
	if err != nil || roleIdx < 1 || roleIdx > len(roles) {
		log.Fatalf("❌ Invalid selection. Please enter a number between 1 and %d.", len(roles))
	}

	selectedRoleID := roles[roleIdx-1].ID
	fmt.Printf("\n[i] Assigning role: %s...\n", roles[roleIdx-1].Name)

	// 5. Fetch the Server IP required for the Visa
	serverIP := storage.GetConfig(db, "vpn_endpoint")
	if serverIP == "" {
		log.Fatal("❌ Server public IP not configured. Run setup again.")
	}

	// 6. Execute the Engine: Assign IP, update DB, inject iptables, add to WireGuard
	approvalData, err := auth.EnrollStandardUser(db, connReq, selectedRoleID, serverIP)
	if err != nil {
		log.Fatalf("❌ Enrollment failed: %v", err)
	}

	// 7. Generate the Visa Token
	finalToken, err := auth.EncodeConnectionApproval(approvalData)
	if err != nil {
		log.Fatalf("❌ Failed to encode Visa token: %v", err)
	}

	// 8. Output Success
	fmt.Println("\n✅ Client Successfully Provisioned!")
	fmt.Printf("   Assigned IP: %s\n", approvalData.AssignedIP)
	fmt.Println("\n==================================================")
	fmt.Println("      SEND THIS VISA TOKEN TO THE CLIENT:         ")
	fmt.Println("==================================================")
	fmt.Println(finalToken)
	fmt.Println("==================================================")
}

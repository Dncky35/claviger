package cli

import (
	"claviger-client/internal/auth"
	"claviger-client/internal/config"
	"fmt"
	"log"
	"strings"
)

func HandleApprove(vault *config.ClientVault, tokenString string) {
	fmt.Println("⏳ Decoding Server Visa...")
	approval, err := auth.DecodeApprovalToken(tokenString)
	if err != nil {
		log.Fatalf("❌ Invalid Visa token: %v", err)
	}

	profile, exists := vault.Profiles[vault.ActiveProfileID]
	if !exists || profile.Status != "pending_approval" {
		log.Fatalf("❌ No pending server request found. Run 'claviger generate' first.")
	}

	profile.AssignedIP = approval.AssignedIP
	profile.ServerKey = approval.ServerPubKey
	profile.ServerEndpoint = approval.ServerEndpoint
	profile.DNS = approval.DNS
	profile.BaseSubnet = approval.BaseSubnet
	profile.Status = "active"
	profile.HubPort = approval.HubPort

	serverIP := strings.Split(approval.ServerEndpoint, ":")[0]
	profile.Name = fmt.Sprintf("%s", serverIP)

	if err := config.Save(vault); err != nil {
		log.Fatalf("❌ Failed to save vault: %v", err)
	}

	fmt.Printf("✅ Visa Accepted! You are now enrolled in: %s\n", profile.Name)
	fmt.Println("Run 'claviger connect' to establish the tunnel.")
}

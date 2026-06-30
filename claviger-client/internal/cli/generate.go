package cli

import (
	"claviger-client/internal/auth"
	"claviger-client/internal/config"
	"claviger-client/internal/vpn"
	"fmt"
	"log"
	"os"
	"runtime"

	"github.com/google/uuid"
)

func HandleGenerate(vault *config.ClientVault) {
	fmt.Println("⏳ Generating cryptographic keys...")
	privKey, pubKey, err := vpn.GenerateKeys()
	if err != nil {
		log.Fatalf("❌ Failed to generate keys: %v", err)
	}

	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "Unknown-Desktop"
	}

	if vault.DeviceID == "" {
		vault.DeviceID = uuid.New().String()
	}

	newProfileID := uuid.New().String()
	newProfile := &config.ServerProfile{
		ID:         newProfileID,
		Name:       "Pending Server...",
		PrivateKey: privKey,
		PublicKey:  pubKey,
		Status:     "pending_approval",
	}

	if vault.Profiles == nil {
		vault.Profiles = make(map[string]*config.ServerProfile)
	}

	vault.Profiles[newProfileID] = newProfile
	vault.ActiveProfileID = newProfileID

	if err := config.Save(vault); err != nil {
		log.Fatalf("❌ Failed to save vault: %v", err)
	}

	token, err := auth.GenerateRequestToken(pubKey, hostname, runtime.GOOS, vault.DeviceID)
	if err != nil {
		log.Fatalf("❌ Failed to build request token: %v", err)
	}

	fmt.Println("\n✅ PASSPORT GENERATED SUCCESSFULLY")
	fmt.Println("Send this token to your Network Administrator:")
	fmt.Println("---------------------------------------------------")
	fmt.Println(token)
	fmt.Println("---------------------------------------------------")
}

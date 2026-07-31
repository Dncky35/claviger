package cmd

import (
	"bytes"
	"claviger-server/storage"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/google/uuid"
)

func RunCreateNode(db *sql.DB, masterIP string, subServerIP string) error {
	// 1. Generate local key if it doesn't exist
	key := storage.GetConfig(db, "node_secret")
	if key == "" {
		key = fmt.Sprintf("clvg_node_%s", uuid.New().String())
		if err := storage.SetConfig(db, "node_secret", key); err != nil {
			return fmt.Errorf("failed to store local node key: %v", err)
		}
	}

	// 2. Save Master's VPN IP
	if err := storage.SetConfig(db, "master_vpn_ip", masterIP); err != nil {
		return fmt.Errorf("failed to store master vpn ip: %v", err)
	}

	// 3. Enable public hub mode so the server binds to 0.0.0.0 on startup,
	// allowing the Master to proxy commands while keeping internal gateway rules intact.
	if err := storage.SetConfig(db, "public_hub", "true"); err != nil {
		return fmt.Errorf("failed to set public_hub config: %v", err)
	}

	// 4. Build registration payload
	payload := map[string]string{
		"vpn_ip":   subServerIP,
		"node_key": key,
	}
	jsonBytes, _ := json.Marshal(payload)

	hubPort := storage.GetConfig(db, "hub_port")
	if hubPort == "" {
		return fmt.Errorf("hub port not configured")
	}

	// 5. Post to Master over WireGuard IP
	masterURL := fmt.Sprintf("http://%s:%s/api/sub-servers/register", masterIP, hubPort)
	resp, err := http.Post(masterURL, "application/json", bytes.NewBuffer(jsonBytes))
	if err != nil {
		return fmt.Errorf("failed to register with master: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("master rejected registration with status: %d", resp.StatusCode)
	}

	log.Println("✅ Node registration request sent to Master successfully.")
	fmt.Println("\n---------------------------------------------------")
	fmt.Println("⚠️  ACTION REQUIRED: Public Hub mode enabled.")
	fmt.Println("   Please restart your Sub-server daemon to apply changes:")
	fmt.Println("   sudo systemctl restart claviger-server (or restart your binary)")
	fmt.Println("---------------------------------------------------")

	return nil
}

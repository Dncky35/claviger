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

	// 2. Build registration payload (Now using vpn_ip)
	payload := map[string]string{
		"vpn_ip":   subServerIP,
		"node_key": key,
	}
	jsonBytes, _ := json.Marshal(payload)

	hubPort := storage.GetConfig(db, "hub_port")
	if hubPort == "" {
		return fmt.Errorf("hub port not configured")
	}

	storage.SetConfig(db, "master_vpn_ip", masterIP)

	// 3. Post to Master over WireGuard IP
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
	return nil
}

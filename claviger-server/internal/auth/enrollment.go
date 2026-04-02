package auth

import (
	"claviger-server/storage"
	"database/sql"
	"fmt"
	"log"
	"net"

	"github.com/google/uuid"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// EnrollFirstAdmin handles the precise offline provisioning of the first Root Admin during setup.
// It completely bypasses the web server and injects the admin directly into the database and kernel.
func EnrollFirstAdmin(db *sql.DB, req *ConnectionRequest, serverPublicIP string) (*ConnectionApproval, error) {
	// 1. Hardcode the First Admin IP (The Hub is .1, Admin gets .2)
	assignIP := "10.8.0.2"
	clientID := req.DeviceID //uuid.New().String()

	// 2. Save the Admin safely into the database as ACTIVE
	_, err := db.Exec(`
		INSERT INTO clients (id, name, public_key, ip_address, role_id, platform, device_id, status) 
		VALUES (?, ?, ?, ?, 'admin', ?, ?, 'active')`,
		clientID, req.Hostname, req.PublicKey, assignIP, req.Platform, req.DeviceID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to save admin to database (key might already exist): %v", err)
	}

	// 3. Hot-Inject the Admin directly into the Linux Kernel (WireGuard)
	wg, err := wgctrl.New()
	if err != nil {
		return nil, fmt.Errorf("failed to open WireGuard control: %v", err)
	}
	defer wg.Close()

	pubKey, parseErr := wgtypes.ParseKey(req.PublicKey)
	if parseErr != nil {
		return nil, fmt.Errorf("invalid public key format from client: %v", parseErr)
	}

	_, ipNet, _ := net.ParseCIDR(assignIP + "/32")

	err = wg.ConfigureDevice("wg0", wgtypes.Config{
		Peers: []wgtypes.PeerConfig{{
			PublicKey:         pubKey,
			ReplaceAllowedIPs: true,
			AllowedIPs:        []net.IPNet{*ipNet},
		}},
	})
	if err != nil {
		log.Printf("⚠️ Warning: Failed to inject Admin into kernel (is wg0 running?): %v", err)
	}

	// 4. Fetch the Server's configuration to build the Approval token
	serverPubKey := storage.GetConfig(db, "wg_public_key")
	wgPort := storage.GetConfig(db, "wg_port")
	if wgPort == "" {
		wgPort = "51820"
	}

	// 5. Construct and return the exact data the Client needs to connect!
	approval := &ConnectionApproval{
		Role:           "admin",
		AssignedIP:     assignIP,
		ServerPubKey:   serverPubKey,
		ServerEndpoint: fmt.Sprintf("%s:%s", serverPublicIP, wgPort),
	}

	return approval, nil
}

// GetNextAvailableIP scans the database and finds the next empty 10.8.0.x address
func GetNextAvailableIP(db *sql.DB) (string, error) {
	// Start at 10.8.0.2 because .1 is the Claviger Hub
	for i := 2; i <= 254; i++ {
		testIP := fmt.Sprintf("10.8.0.%d", i)
		var exists bool
		err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM clients WHERE ip_address = ?)", testIP).Scan(&exists)
		if err != nil {
			return "", err
		}
		if !exists {
			return testIP, nil
		}
	}
	return "", fmt.Errorf("subnet full: no IP addresses available")
}

// EnrollStandardUser handles the UI registration flow for adding a new device
func EnrollStandardUser(db *sql.DB, req *ConnectionRequest, roleID string, serverPublicIP string) (*ConnectionApproval, error) {
	// 1. Duplicate Key Check
	var exists bool
	db.QueryRow("SELECT EXISTS(SELECT 1 FROM clients WHERE public_key = ?)", req.PublicKey).Scan(&exists)
	if exists {
		return nil, fmt.Errorf("this device's public key is already registered on the network")
	}

	db.QueryRow("SELECT EXISTS(SELECT 1 FROM clients WHERE device_id = ?)", req.DeviceID).Scan(&exists)
	if exists {
		return nil, fmt.Errorf("this device is already registered on the network")
	}

	// 2. IP Address Management (IPAM)
	assignIP, err := GetNextAvailableIP(db)
	if err != nil {
		return nil, fmt.Errorf("network full: %v", err)
	}

	clientID := uuid.New().String()

	// 3. Save to Database as ACTIVE
	_, err = db.Exec(`
		INSERT INTO clients (id, name, public_key, ip_address, role_id, platform, device_id, status) 
		VALUES (?, ?, ?, ?, ?, ?, ?, 'active')`,
		clientID, req.Hostname, req.PublicKey, assignIP, roleID, req.Platform, req.DeviceID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to save client to database: %v", err)
	}

	// 4. Hot-Inject into Linux Kernel (WireGuard)
	wg, err := wgctrl.New()
	if err == nil {
		defer wg.Close()
		pubKey, _ := wgtypes.ParseKey(req.PublicKey)
		_, ipNet, _ := net.ParseCIDR(assignIP + "/32")

		wg.ConfigureDevice("wg0", wgtypes.Config{
			Peers: []wgtypes.PeerConfig{{
				PublicKey:         pubKey,
				ReplaceAllowedIPs: true,
				AllowedIPs:        []net.IPNet{*ipNet},
			}},
		})
	} else {
		log.Printf("⚠️ Warning: Failed to inject into kernel: %v", err)
	}

	// 5. Build the Server Config Data
	serverPubKey := storage.GetConfig(db, "wg_public_key")
	wgPort := storage.GetConfig(db, "wg_port")
	if wgPort == "" {
		wgPort = "51820"
	}

	// 6. Generate the Approval Token
	approval := &ConnectionApproval{
		Role:           roleID,
		AssignedIP:     assignIP,
		ServerPubKey:   serverPubKey,
		ServerEndpoint: fmt.Sprintf("%s:%s", serverPublicIP, wgPort),
	}

	return approval, nil
}

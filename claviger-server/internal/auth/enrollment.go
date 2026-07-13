package auth

import (
	"bytes"
	"claviger-server/storage"
	"database/sql"
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"strings"
	"text/template"

	"github.com/google/uuid"
	"github.com/skip2/go-qrcode"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// EnrollFirstAdmin handles the precise offline provisioning of the first Root Admin during setup.
// It completely bypasses the web server and injects the admin directly into the database and kernel.
func EnrollFirstAdmin(db *sql.DB, req *ConnectionRequest, serverPublicIP string) (*ConnectionApproval, error) {
	// 1. Dynamically find the next available IP instead of hardcoding!
	assignIP, err := GetNextAvailableIP(db)
	if err != nil {
		return nil, fmt.Errorf("failed to assign IP to admin (network full): %v", err)
	}

	clientID := req.DeviceID //uuid.New().String()

	// 2. Save the Admin safely into the database as ACTIVE
	_, err = db.Exec(`
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

	// Clean up any hidden spaces or newlines from the DB or arguments
	customEndpoint := strings.TrimSpace(storage.GetConfig(db, "vpn_endpoint"))
	cleanServerIP := strings.TrimSpace(serverPublicIP)

	// If the custom endpoint is empty, use the detected server IP as a fallback
	if customEndpoint == "" {
		customEndpoint = cleanServerIP // Fallback to raw IP
	}

	// 🚨 FAILSAFE: If it is STILL empty, abort immediately! 🚨
	if customEndpoint == "" {
		return nil, fmt.Errorf("CRITICAL ERROR: Cannot generate connection token. The Server IP/Domain is completely empty")
	}

	// 🎯 ADAPTIVE HUB IP
	hubIP := storage.GetConfig(db, "hub_ip")
	if hubIP == "" {
		hubIP = "10.8.0.1"
	}

	// Calculate the /24 Base Subnet dynamically (e.g., "10.8.0.1" -> "10.8.0.0/24")
	baseSubnet := "10.8.0.0/24" // Safe fallback
	lastDot := strings.LastIndex(hubIP, ".")
	if lastDot != -1 {
		baseSubnet = hubIP[:lastDot] + ".0/24"
	}

	// 🎯 ADAPTIVE DNS
	dnsSetting := hubIP // AdGuard runs directly on the Hub IP!
	if storage.GetConfig(db, "app_adguard_port") == "" {
		dnsSetting = "1.1.1.1, 1.0.0.1" // AdGuard missing, use Cloudflare
	}

	// 6. Generate the Approval Token
	approval := &ConnectionApproval{
		Role:           "admin",
		AssignedIP:     assignIP,
		ServerPubKey:   serverPubKey,
		ServerEndpoint: fmt.Sprintf("%s:%s", customEndpoint, wgPort),
		DNS:            dnsSetting, // 🎯 Now dynamically uses the Hub IP
		BaseSubnet:     baseSubnet, // 🎯 Now dynamically calculated
	}

	return approval, nil
}

// GetNextAvailableIP scans the database and finds the next empty IP address in the Hub's subnet
func GetNextAvailableIP(db *sql.DB) (string, error) {
	// 1. Fetch the Hub IP dynamically
	hubIP := storage.GetConfig(db, "hub_ip")
	if hubIP == "" {
		hubIP = "10.8.0.1" // Safe fallback
	}

	// 2. Extract the base network prefix (e.g., "10.8.0.1" -> "10.8.0")
	basePrefix := "10.8.0"
	lastDot := strings.LastIndex(hubIP, ".")
	if lastDot != -1 {
		basePrefix = hubIP[:lastDot]
	}

	// 3. Start at .2 (because .1 is the Hub) and scan up to .254
	for i := 2; i <= 254; i++ {
		testIP := fmt.Sprintf("%s.%d", basePrefix, i)
		var exists bool

		err := db.QueryRow("SELECT EXISTS(SELECT 1 FROM clients WHERE ip_address = ?)", testIP).Scan(&exists)
		if err != nil {
			return "", err
		}

		if !exists {
			return testIP, nil // Found an empty slot!
		}
	}

	return "", fmt.Errorf("subnet %s.0/24 is full: no IP addresses available", basePrefix)
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

	customEndpoint := storage.GetConfig(db, "vpn_endpoint")
	if customEndpoint == "" {
		customEndpoint = serverPublicIP // Fallback to raw IP
	}

	// 🎯 ADAPTIVE HUB IP
	hubIP := storage.GetConfig(db, "hub_ip")
	if hubIP == "" {
		hubIP = "10.8.0.1"
	}

	// Calculate the /24 Base Subnet dynamically (e.g., "10.8.0.1" -> "10.8.0.0/24")
	baseSubnet := "10.8.0.0/24" // Safe fallback
	lastDot := strings.LastIndex(hubIP, ".")
	if lastDot != -1 {
		baseSubnet = hubIP[:lastDot] + ".0/24"
	}

	// 🎯 ADAPTIVE DNS
	dnsSetting := hubIP // AdGuard runs directly on the Hub IP!
	if storage.GetConfig(db, "app_adguard_port") == "" {
		dnsSetting = "1.1.1.1, 1.0.0.1" // AdGuard missing, use Cloudflare
	}

	hubPort := storage.GetConfig(db, "hub_port")
	if hubPort == "" {
		hubPort = "18080" // Safe fallback just in case
	}

	// 6. Generate the Approval Token
	approval := &ConnectionApproval{
		Role:           roleID,
		AssignedIP:     assignIP,
		ServerPubKey:   serverPubKey,
		ServerEndpoint: fmt.Sprintf("%s:%s", customEndpoint, wgPort),
		DNS:            dnsSetting, // 🎯 Now dynamically uses the Hub IP
		BaseSubnet:     baseSubnet, // 🎯 Now dynamically calculated
		HubPort:        hubPort,
	}

	return approval, nil
}

// --------------------------------------------------
// ------------ MOBILE ENROLLMENT FLOW BELOW ------------
// --------------------------------------------------

// MobileEnrollReq represents the incoming request from the UI to add a phone
type MobileEnrollReq struct {
	Name             string `json:"name"`
	Role             string `json:"role"`               // e.g., "standard_user", "admin"
	UseGlobalRouting bool   `json:"use_global_routing"` // 🎯 NEW: User selects routing mode
}

// MobileEnrollResp contains the Base64 image and safe data (NO PRIVATE KEYS!)
type MobileEnrollResp struct {
	QRCodeBase64 string `json:"qr_code_base64"`
	PublicKey    string `json:"public_key"`
	AssignedIP   string `json:"assigned_ip"`
}

// wgConfigTemplate is the standard WireGuard format for iOS/Android apps
const wgConfigTemplate = `[Interface]
PrivateKey = {{.PrivateKey}}
Address = {{.ClientIP}}/32
DNS = {{.DNSIP}}
MTU = 1280

[Peer]
PublicKey = {{.ServerPubKey}}
Endpoint = {{.Endpoint}}
AllowedIPs = {{.AllowedIPs}}
PersistentKeepalive = 25`

// TemplateData holds the variables for the WireGuard config template
type TemplateData struct {
	PrivateKey   string
	ClientIP     string
	DNSIP        string
	ServerPubKey string
	Endpoint     string
	AllowedIPs   string
	MTU          int
}

// EnrollMobileDevice generates a keypair, saves the public key, injects it into wg0,
// and returns a Base64 QR Code. The Private Key is destroyed after this function returns.
func EnrollMobileDevice(db *sql.DB, req *MobileEnrollReq, serverPublicIP string) (*MobileEnrollResp, error) {
	// 1. GENERATE CRYPTOGRAPHIC KEYS (The "Burn After Reading" keys)
	privKey, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate wireguard keys: %v", err)
	}
	pubKey := privKey.PublicKey()

	// 2. IP ADDRESS MANAGEMENT
	assignIP, err := GetNextAvailableIP(db)
	if err != nil {
		return nil, fmt.Errorf("network full: %v", err)
	}

	clientID := uuid.New().String()

	// 3. SAVE TO DATABASE (Only the Public Key!)
	// We use the clientID as the device_id since QR scans don't have hardware IDs
	_, err = db.Exec(`
		INSERT INTO clients (id, name, public_key, ip_address, role_id, platform, device_id, status) 
		VALUES (?, ?, ?, ?, ?, 'mobile', ?, 'active')`,
		clientID, req.Name, pubKey.String(), assignIP, req.Role, clientID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to save mobile client to database: %v", err)
	}

	// 4. HOT-INJECT INTO LINUX KERNEL
	wg, err := wgctrl.New()
	if err == nil {
		defer wg.Close()
		_, ipNet, _ := net.ParseCIDR(assignIP + "/32")

		err = wg.ConfigureDevice("wg0", wgtypes.Config{
			Peers: []wgtypes.PeerConfig{{
				PublicKey:         pubKey,
				ReplaceAllowedIPs: true,
				AllowedIPs:        []net.IPNet{*ipNet},
			}},
		})
		if err != nil {
			log.Printf("⚠️ Warning: Failed to inject mobile peer into wg0: %v", err)
		}
	} else {
		log.Printf("⚠️ Warning: Failed to open wgctrl for mobile peer: %v", err)
	}

	// 5. FETCH SERVER CONFIGURATION
	serverPubKey := storage.GetConfig(db, "wg_public_key")
	wgPort := storage.GetConfig(db, "wg_port")
	if wgPort == "" {
		wgPort = "51820"
	}

	customEndpoint := strings.TrimSpace(storage.GetConfig(db, "vpn_endpoint"))
	if customEndpoint == "" {
		customEndpoint = strings.TrimSpace(serverPublicIP)
	}

	// 🎯 FIX 1: DYNAMIC ROUTING (Split vs Global)
	// Fetch the base subnet from the database (fallback to 10.8.0.0/24 if not found)
	baseSubnet := storage.GetConfig(db, "vpn_subnet")
	if baseSubnet == "" {
		baseSubnet = "10.8.0.0/24"
	}

	var allowedIPs string
	if req.UseGlobalRouting {
		// FULL TUNNEL: Route absolutely everything through the VPS
		allowedIPs = "0.0.0.0/0, ::/0"
	} else {
		// SPLIT TUNNEL (ZERO TRUST MODE): Only route traffic meant for the private network
		// This saves mobile battery and avoids carrier throttling!
		allowedIPs = baseSubnet
	}

	// 🎯 FIX 2: SMART DNS FALLBACK
	// If AdGuard is NOT installed yet, fallback to Cloudflare (1.1.1.1) so the internet still works!
	dnsIP := "10.8.0.1"
	if storage.GetConfig(db, "app_adguard_port") == "" {
		// AdGuard port isn't in the DB, meaning it's not installed. Use public DNS.
		dnsIP = "1.1.1.1, 1.0.0.1"

		// CRITICAL EDGE CASE: If they chose Split Tunnel, but use public DNS,
		// we must append the DNS IPs to the AllowedIPs, otherwise DNS queries will fail!
		if !req.UseGlobalRouting {
			allowedIPs = fmt.Sprintf("%s, 1.1.1.1/32, 1.0.0.1/32", baseSubnet)
		}
	}

	// 6. BUILD THE RAW CONFIG TEXT FOR THE QR CODE
	tmpl, err := template.New("wgconf").Parse(wgConfigTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to parse wg template: %v", err)
	}

	var confText bytes.Buffer
	data := TemplateData{
		PrivateKey:   privKey.String(),
		ClientIP:     assignIP,
		DNSIP:        dnsIP,
		ServerPubKey: serverPubKey,
		Endpoint:     fmt.Sprintf("%s:%s", customEndpoint, wgPort),
		AllowedIPs:   allowedIPs,
	}
	if err := tmpl.Execute(&confText, data); err != nil {
		return nil, fmt.Errorf("failed to inject wg config: %v", err)
	}

	// 7. RENDER THE QR CODE IN MEMORY
	// qrcode.High provides 30% error recovery, perfect for phone cameras!
	qrBytes, err := qrcode.Encode(confText.String(), qrcode.High, 256)
	if err != nil {
		return nil, fmt.Errorf("failed to generate QR code: %v", err)
	}

	// Convert raw PNG to Base64 for the React frontend
	base64Image := "data:image/png;base64," + base64.StdEncoding.EncodeToString(qrBytes)

	// 8. RETURN SUCCESS
	return &MobileEnrollResp{
		QRCodeBase64: base64Image,
		PublicKey:    pubKey.String(),
		AssignedIP:   assignIP,
	}, nil
}

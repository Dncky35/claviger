package vpn

import (
	"claviger-client/internal/config"
	"fmt"
	"os"
	"path/filepath"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// GenerateKeys creates a fresh, highly secure Curve25519 keypair.
// It returns (PrivateKey, PublicKey, Error)
func GenerateKeys() (string, string, error) {
	key, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		return "", "", err
	}

	// The Private Key never leaves the client's laptop!
	privateKey := key.String()

	// The Public Key is mathematically derived and safe to share with the Hub.
	publicKey := key.PublicKey().String()

	return privateKey, publicKey, nil
}

// GetConfigPath determines where to safely store the temporary config file.
// We use the OS's native temporary directory so it cleans itself up.
func GetConfigPath() string {
	return filepath.Join(os.TempDir(), "claviger.conf")
}

// WriteConfigFile converts the Vault data into a valid WireGuard configuration file
func WriteConfigFile(vault *config.ClientVault) (string, error) {
	// 1. Create the WireGuard configuration string
	// Notice AllowedIPs = 10.8.0.0/24. This is a "Split Tunnel".
	// It routes only Hub traffic through the VPN, leaving normal internet alone.
	confData := fmt.Sprintf(`[Interface]
PrivateKey = %s
Address = %s/32

[Peer]
PublicKey = %s
Endpoint = %s
AllowedIPs = 10.8.0.0/24
PersistentKeepalive = 25
`,
		vault.PrivateKey,
		vault.AssignedIP,
		vault.ServerKey,
		vault.ServerEndpoint,
	)

	// 2. Get the safe path to save the file
	configPath := GetConfigPath()

	// 3. Write the file to disk
	// We use '0600' permissions so ONLY the current user/admin can read the private key.
	err := os.WriteFile(configPath, []byte(confData), 0600)
	if err != nil {
		return "", fmt.Errorf("failed to write VPN config: %v", err)
	}

	return configPath, nil
}

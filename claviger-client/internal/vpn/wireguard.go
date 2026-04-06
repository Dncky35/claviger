package vpn

import (
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

	// The Public Key is mathematically derived and safe to share with the Admin.
	publicKey := key.PublicKey().String()

	return privateKey, publicKey, nil
}

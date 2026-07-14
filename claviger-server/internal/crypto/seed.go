package crypto

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"

	"github.com/tyler-smith/go-bip39"
	"golang.org/x/crypto/hkdf"
)

// ClavigerKeys holds our two mathematically linked root keys.
type ClavigerKeys struct {
	WireGuardPrivateKey []byte // Exactly 32 bytes for Curve25519
	AESBackupKey        []byte // Exactly 32 bytes for AES-256
}

// GenerateNewMnemonic creates a fresh 12-word BIP39 seed phrase.
// This is ONLY called once during the initial server provisioning.
func GenerateNewMnemonic() (string, error) {
	// Generate 128 bits of entropy (yields 12 words)
	entropy, err := bip39.NewEntropy(128)
	if err != nil {
		return "", fmt.Errorf("failed to generate entropy: %v", err)
	}

	mnemonic, err := bip39.NewMnemonic(entropy)
	if err != nil {
		return "", fmt.Errorf("failed to generate mnemonic: %v", err)
	}

	return mnemonic, nil
}

// DeriveKeys takes the 12-word seed phrase and deterministically expands it
// into two separate, secure 32-byte keys using HKDF-SHA256.
func DeriveKeys(mnemonic string) (*ClavigerKeys, error) {
	if !bip39.IsMnemonicValid(mnemonic) {
		return nil, errors.New("invalid seed phrase provided")
	}

	// 1. Convert the mnemonic to a standard BIP39 seed (no passphrase used)
	seed := bip39.NewSeed(mnemonic, "")

	// 2. Setup the HKDF (HMAC-based Key Derivation Function)
	// We use SHA-256 as the hash, the BIP39 seed as the secret, and a custom info string.
	// The 'info' string binds these keys specifically to the Claviger application context.
	hkdfReader := hkdf.New(sha256.New, seed, nil, []byte("claviger-zero-trust-gateway-root"))

	// 3. We need 64 bytes total (32 for WireGuard, 32 for AES)
	keyMaterial := make([]byte, 64)
	if _, err := io.ReadFull(hkdfReader, keyMaterial); err != nil {
		return nil, fmt.Errorf("HKDF derivation failed: %v", err)
	}

	// 4. Split the derived material perfectly in half
	keys := &ClavigerKeys{
		WireGuardPrivateKey: keyMaterial[:32], // First 32 bytes
		AESBackupKey:        keyMaterial[32:], // Second 32 bytes
	}

	return keys, nil
}

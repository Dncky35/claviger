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
	AESBackupKey []byte // Exactly 32 bytes for AES-256
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
func DeriveAESKey(mnemonic string) ([]byte, error) {
	if !bip39.IsMnemonicValid(mnemonic) {
		return nil, errors.New("invalid seed phrase provided")
	}

	// 1. Convert the mnemonic to a standard BIP39 seed (no passphrase used)
	seed := bip39.NewSeed(mnemonic, "")

	// 2. Setup the HKDF
	// We bind to a specific "claviger-backup-encryption" context to ensure
	// this key is cryptographically isolated from any other app secrets.
	hkdfReader := hkdf.New(sha256.New, seed, nil, []byte("claviger-backup-encryption"))

	// 3. We need exactly 32 bytes for an AES-256 key
	aesKey := make([]byte, 32)
	if _, err := io.ReadFull(hkdfReader, aesKey); err != nil {
		return nil, fmt.Errorf("HKDF derivation failed: %v", err)
	}

	return aesKey, nil
}

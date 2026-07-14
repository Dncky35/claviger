package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"io"
	"os"
)

// EncryptFile encrypts the source file using the AES key and saves to destPath
func EncryptFile(srcPath, destPath string, key []byte) error {
	plaintext, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}

	// Create a random nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return err
	}

	// Seal the data (Encrypt + Authenticate)
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)

	return os.WriteFile(destPath, ciphertext, 0600)
}

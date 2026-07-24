package security

import (
	"claviger-server/storage"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"log"

	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"
)

// ---------------------------------------------------------
// 1. TOTP Generation & Validation
// ---------------------------------------------------------

// GenerateTOTPSecret creates a new secret for a user.
// Returns the base32 secret and the URL used to generate a QR code.
func GenerateTOTPSecret(clientEmailOrName string) (string, string, error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "Claviger Gateway",
		AccountName: clientEmailOrName,
	})
	if err != nil {
		return "", "", err
	}

	// key.Secret() -> "JBSWY3DPEHPK3PXP" (Store this in the DB)
	// key.URL()    -> "otpauth://totp/Claviger...?" (Send to frontend for QR)
	return key.Secret(), key.URL(), nil
}

// ValidateTOTPCode checks if the 6-digit user input matches the current time window.
func ValidateTOTPCode(passcode string, secret string) bool {
	// totp.Validate automatically handles the current time and a slight drift window
	return totp.Validate(passcode, secret)
}

// ---------------------------------------------------------
// 2. Recovery Key Generation & Hashing
// ---------------------------------------------------------

// GenerateRecoveryKeys creates cryptographically secure random backup codes.
// Returns 8 plain-text keys (e.g., "a1b2c3d4-e5f6g7h8")
func GenerateRecoveryKeys() ([]string, error) {
	keys := make([]string, 8)
	for i := 0; i < 8; i++ {
		bytes := make([]byte, 8) // 8 bytes = 16 hex chars
		if _, err := rand.Read(bytes); err != nil {
			return nil, err
		}
		// Format it nicely: "xxxx-xxxx"
		hexStr := hex.EncodeToString(bytes)
		keys[i] = hexStr[:4] + "-" + hexStr[4:8] + "-" + hexStr[8:12]
	}
	return keys, nil
}

// HashRecoveryKeys takes plain text keys, bcrypts them, and returns a JSON string
// ready to be safely stored in the `recovery_keys` database column.
func HashRecoveryKeys(plainKeys []string) (string, error) {
	var hashedKeys []string

	for _, key := range plainKeys {
		hash, err := bcrypt.GenerateFromPassword([]byte(key), bcrypt.DefaultCost)
		if err != nil {
			log.Printf("⚠️ Error hashing recovery key: %v", err)
			return "", err
		}
		hashedKeys = append(hashedKeys, string(hash))
	}

	// Convert the array of hashes into a JSON string for SQLite
	jsonHashes, err := json.Marshal(hashedKeys)
	if err != nil {
		return "", err
	}

	return string(jsonHashes), nil
}

// VerifyRecoveryKey checks if a user-provided recovery key matches any stored hash.
func VerifyRecoveryKey(providedKey string, dbHashesJSON string) bool {
	var storedHashes []string
	if err := json.Unmarshal([]byte(dbHashesJSON), &storedHashes); err != nil {
		return false
	}

	for _, hash := range storedHashes {
		// CompareHashAndPassword returns nil if they match
		if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(providedKey)); err == nil {
			return true
		}
	}
	return false
}

func EnsureJWTSecret(db *sql.DB) []byte {
	secretStr := storage.GetConfig(db, "jwt_secret")

	// 2. If it exists, return it immediately
	if secretStr != "" {
		return []byte(secretStr)
	}

	// 3. If it does NOT exist (first boot), generate a secure 32-byte random key
	log.Println("🌱 First boot detected: Generating secure JWT secret...")
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		log.Fatalf("❌ Failed to generate cryptographic random bytes: %v", err)
	}

	// 4. Convert it to a hex string to easily store it in the SQLite config table
	newSecretStr := hex.EncodeToString(key)
	storage.SetConfig(db, "jwt_secret", newSecretStr)

	return []byte(newSecretStr)
}

package system

var (
	// Global variable accessible to your cron tasks
	// This lives in RAM only, never written to disk
	ActiveBackupKey []byte
)

// SetBackupKey populates the key at startup
func SetBackupKey(key []byte) {
	ActiveBackupKey = key
}

// GetActiveAESKey returns the key safely
func GetActiveAESKey() []byte {
	return ActiveBackupKey
}

package access

import (
	"bufio"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

type SSHKey struct {
	KeyType string `json:"key_type"`
	Preview string `json:"preview"`
	Comment string `json:"comment"`
	Raw     string `json:"raw"`
}

// getSSHFilePath safely finds the ~/.ssh/authorized_keys file of the TARGET user
func getSSHFilePath() (string, error) {
	// 1. Check for an explicit application override (perfect for systemd)
	targetUsername := os.Getenv("CLAVIGER_SSH_USER")

	// 2. If not overridden, check if run via interactive sudo
	if targetUsername == "" {
		targetUsername = os.Getenv("SUDO_USER")
	}

	var targetUser *user.User
	var err error

	// If we found a specific non-root user, look up their home directory
	if targetUsername != "" && targetUsername != "root" {
		targetUser, err = user.Lookup(targetUsername)
	} else {
		// 3. Fallback: No override or sudo environment found, use current process user (root)
		targetUser, err = user.Current()
	}

	if err != nil {
		return "", fmt.Errorf("could not determine target user: %v", err)
	}

	sshDir := filepath.Join(targetUser.HomeDir, ".ssh")

	// SSH strictly requires the directory to be 0700
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		return "", fmt.Errorf("could not create .ssh directory: %v", err)
	}

	return filepath.Join(sshDir, "authorized_keys"), nil
}

// ListKeys reads the authorized_keys file and parses it into structs
func ListKeys() ([]SSHKey, error) {
	path, err := getSSHFilePath()
	if err != nil {
		return nil, err
	}

	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []SSHKey{}, nil // File doesn't exist yet, which is fine
		}
		return nil, err
	}
	defer file.Close()

	var keys []SSHKey
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Ignore empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) >= 2 {
			keyType := parts[0]
			keyData := parts[1]
			comment := ""
			if len(parts) >= 3 {
				comment = strings.Join(parts[2:], " ")
			} else {
				comment = "No Comment"
			}

			// Create a short preview of the long base64 string
			preview := keyData
			if len(keyData) > 30 {
				preview = keyData[:15] + "..." + keyData[len(keyData)-10:]
			}

			keys = append(keys, SSHKey{
				KeyType: keyType,
				Preview: preview,
				Comment: comment,
				Raw:     line,
			})
		}
	}

	return keys, nil
}

// AddKey safely appends a new public key to the file
func AddKey(rawKey string) error {
	rawKey = strings.TrimSpace(rawKey)
	if rawKey == "" {
		return fmt.Errorf("cannot add empty key")
	}

	// Validate it looks like an SSH key (very basic check)
	if !strings.HasPrefix(rawKey, "ssh-") && !strings.HasPrefix(rawKey, "ecdsa-") {
		return fmt.Errorf("invalid SSH key format")
	}

	path, err := getSSHFilePath()
	if err != nil {
		return err
	}

	// Open file in append mode. SSH strictly requires 0600 permissions.
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer file.Close()

	// Ensure we start on a new line
	_, err = file.WriteString("\n" + rawKey + "\n")
	return err
}

// RevokeKey removes any key that matches the exact comment provided
func RevokeKey(commentTarget string) error {
	path, err := getSSHFilePath()
	if err != nil {
		return err
	}

	keys, err := ListKeys()
	if err != nil {
		return err
	}

	// Rewrite the file, keeping only the keys that DO NOT match the comment
	var updatedLines []string
	for _, k := range keys {
		if k.Comment != commentTarget {
			updatedLines = append(updatedLines, k.Raw)
		}
	}

	output := strings.Join(updatedLines, "\n") + "\n"

	// Overwrite the file with the new filtered list
	return os.WriteFile(path, []byte(output), 0600)
}

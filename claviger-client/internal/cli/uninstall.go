package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

func HandleUninstall() {
	fmt.Println("🧹 Starting Claviger data wipe...")

	var pathsToRemove []string

	switch runtime.GOOS {
	case "windows":
		programData := os.Getenv("PROGRAMDATA")
		if programData == "" {
			programData = `C:\ProgramData`
		}
		pathsToRemove = append(pathsToRemove, filepath.Join(programData, "Claviger"))
	case "darwin":
		pathsToRemove = append(pathsToRemove, "/Library/Application Support/Claviger")
	default:
		// Linux: Target both the old and new locations to ensure a clean slate
		pathsToRemove = append(pathsToRemove, "/var/lib/claviger", "/etc/claviger")

		// Optional: Target the user's home directory just in case
		if home, err := os.UserHomeDir(); err == nil {
			pathsToRemove = append(pathsToRemove, filepath.Join(home, ".config", "claviger"))
		}
	}

	successCount := 0
	for _, targetPath := range pathsToRemove {
		if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
			fmt.Printf("🗑️  Removing %s... ", targetPath)
			if err := os.RemoveAll(targetPath); err != nil {
				fmt.Printf("❌ Failed (Permission denied? Run with sudo/Admin)\n")
			} else {
				fmt.Printf("✅\n")
				successCount++
			}
		}
	}

	if successCount > 0 {
		fmt.Println("✅ Claviger configuration and vault data completely wiped.")
	} else {
		fmt.Println("✅ No Claviger data found. System is already clean.")
	}

	// Remind Linux users to use the package manager for the actual executable
	if runtime.GOOS == "linux" {
		fmt.Println("\n⚠️  Note: To remove the application binaries from Linux, run:")
		fmt.Println("    Debian/Ubuntu: sudo apt-get purge claviger-client")
		fmt.Println("    RHEL/CentOS:   sudo yum remove claviger-client")
	}
}

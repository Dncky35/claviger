//go:build windows

package vpn

import (
	_ "embed"
	"log"
	"os"
	"path/filepath"
)

//go:embed wintun.dll
var wintunDLL []byte

// init() runs automatically before the main() function starts
func init() {
	// Find out exactly where the .exe is currently running from
	exePath, err := os.Executable()
	if err != nil {
		log.Printf("⚠️ Could not determine executable path: %v", err)
		return
	}
	exeDir := filepath.Dir(exePath)
	dllPath := filepath.Join(exeDir, "wintun.dll")

	// Check if the DLL is already there. If not, silently extract it!
	if _, err := os.Stat(dllPath); os.IsNotExist(err) {
		log.Println("⚙️ First run detected: Silently extracting Windows Network Driver...")
		err := os.WriteFile(dllPath, wintunDLL, 0644)
		if err != nil {
			log.Printf("❌ Failed to extract wintun.dll: %v", err)
		} else {
			log.Println("✅ Windows Network Driver installed successfully.")
		}
	}
}

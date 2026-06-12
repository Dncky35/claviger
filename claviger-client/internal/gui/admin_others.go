//go:build !windows

package gui

import (
	"log"
	"os"
)

// EnsureAdmin no longer crashes the app on Linux.
// It just logs that we are running in User Space (Client Mode).
func EnsureAdmin() {
	if os.Geteuid() != 0 {
		log.Println("👤 Running Claviger GUI in standard User Mode. Network commands will be delegated to the background daemon.")
	} else {
		log.Println("⚠️ Running Claviger GUI as Root. (Not recommended for Desktop Linux)")
	}
}

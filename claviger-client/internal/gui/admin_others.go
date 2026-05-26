//go:build !windows

package gui

import (
	"log"
	"os"
)

// EnsureAdmin checks for root on Linux/Mac
func EnsureAdmin() {
	if os.Geteuid() != 0 {
		log.Fatalf("❌ Claviger requires Root (sudo) privileges on this operating system.")
	}
}

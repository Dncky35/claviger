//go:build windows

package watchdog

// These are dummy functions so VS Code doesn't throw errors when you develop on Windows.
// The real functions will run when you deploy this to your Linux server.

func checkDiskUsage(thresholdPercent int) {
	// Dummy
}

func checkWireguardInterface() {
	// Dummy
}

func checkUFWStatus() {
	// Dummy
}

func checkDockerHealth() {
	// Dummy
}

func checkRAMUsage(thresholdPercent int) {
	// Dummy
}

func checkCPUUsage(thresholdPercent int) {
	// Dummy
}

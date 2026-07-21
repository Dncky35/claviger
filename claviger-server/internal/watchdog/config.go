package watchdog

// WatchdogConfig holds all the thresholds and toggles for system monitoring.
// This matches exactly what we will store in the SQLite database.
type WatchdogConfig struct {
	// 1. System Health
	NotifyOnDaemonRestart bool
	NotifyOnHighDiskUsage bool
	NotifyOnHighCPUUsage  bool
	CPUWarningThreshold   int  // e.g., 80 (meaning 80%)
	DiskWarningThreshold  int  // e.g., 90 (meaning 90%)
	NotifyOnHighRAMUsage  bool // NEW: Prevent OOM crashes
	RAMWarningThreshold   int  // e.g., 85 (meaning 85%)

	// 2. Security & Perimeter (Crucial for Zero Trust)
	NotifyOnFirewallDrop    bool // NEW: Alerts if UFW is disabled
	NotifyOnWGInterfaceDown bool // NEW: Alerts if wg0 vanishes
	NotifyOnSSHBruteForce   bool // NEW: Integrates with Fail2Ban later

	// 3. Application Ecosystem
	NotifyOnDockerCrash bool
}

package api

import (
	"claviger-server/internal/watchdog"
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
)

// WatchdogConfigPayload represents the JSON payload for the UI.
type WatchdogConfigPayload struct {
	// System Health
	NotifyOnDaemonRestart bool `json:"notify_on_daemon_restart"`
	NotifyOnHighDiskUsage bool `json:"notify_on_high_disk_usage"`
	DiskWarningThreshold  int  `json:"disk_warning_threshold"`
	NotifyOnHighRAMUsage  bool `json:"notify_on_high_ram_usage"`
	RAMWarningThreshold   int  `json:"ram_warning_threshold"`
	NotifyOnHighCPUUsage  bool `json:"notify_on_high_cpu_usage"`
	CPUWarningThreshold   int  `json:"cpu_warning_threshold"`

	// Zero Trust Perimeter
	NotifyOnFirewallDrop    bool `json:"notify_on_firewall_drop"`
	NotifyOnWGInterfaceDown bool `json:"notify_on_wg_interface_down"`
	NotifyOnSSHBruteForce   bool `json:"notify_on_ssh_brute_force"`

	// Application Ecosystem
	NotifyOnDockerCrash bool `json:"notify_on_docker_crash"`
}

// HandleGetWatchdogConfig reads current Watchdog settings and thresholds.
func HandleGetWatchdogConfig(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. Fetch the active configuration using our existing helper
		cfg := watchdog.LoadConfigFromDB(db)

		// 2. Map it to the UI payload
		payload := WatchdogConfigPayload{
			NotifyOnDaemonRestart:   cfg.NotifyOnDaemonRestart,
			NotifyOnHighDiskUsage:   cfg.NotifyOnHighDiskUsage,
			DiskWarningThreshold:    cfg.DiskWarningThreshold,
			NotifyOnHighRAMUsage:    cfg.NotifyOnHighRAMUsage,
			RAMWarningThreshold:     cfg.RAMWarningThreshold,
			NotifyOnHighCPUUsage:    cfg.NotifyOnHighCPUUsage,
			CPUWarningThreshold:     cfg.CPUWarningThreshold,
			NotifyOnFirewallDrop:    cfg.NotifyOnFirewallDrop,
			NotifyOnWGInterfaceDown: cfg.NotifyOnWGInterfaceDown,
			NotifyOnSSHBruteForce:   cfg.NotifyOnSSHBruteForce,
			NotifyOnDockerCrash:     cfg.NotifyOnDockerCrash,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(payload)
	}
}

// HandleSaveWatchdogConfig saves the Watchdog settings into the config table.
func HandleSaveWatchdogConfig(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var payload WatchdogConfigPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
			return
		}

		// Prepare the upsert statement
		stmt, err := db.Prepare(`
			INSERT INTO config (key, value) VALUES (?, ?) 
			ON CONFLICT(key) DO UPDATE SET value = excluded.value;
		`)
		if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		defer stmt.Close()

		// Helper to save boolean as string
		saveBool := func(key string, val bool) {
			stmt.Exec(key, strconv.FormatBool(val))
		}

		// Helper to save integer as string
		saveInt := func(key string, val int) {
			stmt.Exec(key, strconv.Itoa(val))
		}

		// Save System Health rules
		saveBool("watchdog_notify_daemon_restart", payload.NotifyOnDaemonRestart)
		saveBool("watchdog_notify_disk", payload.NotifyOnHighDiskUsage)
		saveInt("watchdog_disk_threshold", payload.DiskWarningThreshold)
		saveBool("watchdog_notify_ram", payload.NotifyOnHighRAMUsage)
		saveInt("watchdog_ram_threshold", payload.RAMWarningThreshold)
		saveBool("watchdog_notify_cpu", payload.NotifyOnHighCPUUsage)
		saveInt("watchdog_cpu_threshold", payload.CPUWarningThreshold)

		// Save Perimeter rules
		saveBool("watchdog_notify_firewall", payload.NotifyOnFirewallDrop)
		saveBool("watchdog_notify_wg", payload.NotifyOnWGInterfaceDown)
		saveBool("watchdog_notify_ssh", payload.NotifyOnSSHBruteForce)

		// Save Docker rules
		saveBool("watchdog_notify_docker_crash", payload.NotifyOnDockerCrash)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"success"}`))
	}
}

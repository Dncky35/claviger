package watchdog

import (
	"context"
	"database/sql"
	"log"
	"strconv"
	"time"
)

// DefaultConfig provides safe fallbacks if the user hasn't set anything in the UI yet
var DefaultConfig = WatchdogConfig{
	NotifyOnDaemonRestart: true,
	NotifyOnDockerCrash:   true,
	NotifyOnHighDiskUsage: true,
	DiskWarningThreshold:  90,
	NotifyOnHighRAMUsage:  true,
	RAMWarningThreshold:   85,
	NotifyOnHighCPUUsage:  false, // CPU spikes often, default to off
	CPUWarningThreshold:   95,
}

type Engine struct {
	Config WatchdogConfig
	ticker *time.Ticker
}

func NewEngine(cfg WatchdogConfig) *Engine {
	return &Engine{
		Config: cfg,
	}
}

func (e *Engine) Start(ctx context.Context) {
	log.Println("Watchdog Engine Started.")

	e.ticker = time.NewTicker(3 * time.Minute)

	go func() {
		for {
			select {
			case <-ctx.Done():
				// The main Claviger daemon is shutting down. Stop the ticker safely.
				log.Println("🛑 Watchdog Engine Shutting Down...")
				e.ticker.Stop()
				return

			case <-e.ticker.C:
				// The ticker fired. Run all active checks!
				e.runChecks()
			}
		}
	}()
}

// runChecks routes to the specific sensor functions based on user config.
func (e *Engine) runChecks() {
	// --- System Health ---
	if e.Config.NotifyOnHighDiskUsage {
		checkDiskUsage(e.Config.DiskWarningThreshold)
	}

	if e.Config.NotifyOnHighRAMUsage {
		checkRAMUsage(e.Config.RAMWarningThreshold)
	}

	if e.Config.NotifyOnHighCPUUsage {
		checkCPUUsage(e.Config.CPUWarningThreshold)
	}

	// // --- Zero Trust Perimeter ---
	if e.Config.NotifyOnWGInterfaceDown {
		checkWireguardInterface()
	}
	if e.Config.NotifyOnFirewallDrop {
		checkUFWStatus()
	}

	// --- Docker Apps ---
	if e.Config.NotifyOnDockerCrash {
		checkDockerHealth()
	}

	if e.Config.NotifyOnSSHBruteForce {
		checkSSHBruteForce()
	}
}

// LoadConfigFromDB reads the settings from your config table
func LoadConfigFromDB(db *sql.DB) WatchdogConfig {
	cfg := DefaultConfig // Start with defaults

	// We define a helper function to query the DB for a specific key
	getVal := func(key string) string {
		var val string
		err := db.QueryRow("SELECT value FROM config WHERE key = ?", key).Scan(&val)
		if err != nil {
			return "" // Key doesn't exist yet
		}
		return val
	}

	// Parse Booleans
	if val := getVal("watchdog_notify_daemon_restart"); val != "" {
		cfg.NotifyOnDaemonRestart = val == "true"
	}
	if val := getVal("watchdog_notify_docker_crash"); val != "" {
		cfg.NotifyOnDockerCrash = val == "true"
	}
	if val := getVal("watchdog_notify_disk"); val != "" {
		cfg.NotifyOnHighDiskUsage = val == "true"
	}
	if val := getVal("watchdog_notify_ram"); val != "" {
		cfg.NotifyOnHighRAMUsage = val == "true"
	}

	// Parse Integers (Thresholds)
	if val := getVal("watchdog_disk_threshold"); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil {
			cfg.DiskWarningThreshold = parsed
		}
	}
	if val := getVal("watchdog_ram_threshold"); val != "" {
		if parsed, err := strconv.Atoi(val); err == nil {
			cfg.RAMWarningThreshold = parsed
		}
	}

	return cfg
}

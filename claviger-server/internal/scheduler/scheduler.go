package scheduler

import (
	"claviger-server/internal/system"
	"claviger-server/network"
	"claviger-server/storage"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/robfig/cron/v3"
)

// Task represents a background job for the UI and the Engine
type Task struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
	Schedule    string `json:"-"` // Hidden from JSON, e.g., "@daily"
	Enabled     bool   `json:"enabled"`
	LastRun     string `json:"last_run"`
	NextRun     string `json:"next_run"`
	LastStatus  string `json:"last_status"` // "pending", "success", "failed"

	entryID cron.EntryID
	job     func() error
}

var (
	engine *cron.Cron
	Tasks  map[string]*Task
)

// Start initializes the engine and registers our default tasks
func Start(db *sql.DB) {
	engine = cron.New()
	Tasks = make(map[string]*Task)

	// ==========================================
	// 🛠️ REGISTER YOUR AUTOMATED TASKS HERE
	// ==========================================

	RegisterTask("db-backup", "Database Backup", "Creates a safe snapshot of your SQLite database.", "💾", "@daily", true, func() error {
		log.Println("[Cron] 💾 Executing Database Backup...")

		// 1. Load the seed we saved during install
		key := system.GetActiveAESKey()
		if len(key) != 32 {
			return fmt.Errorf("backup aborted: key not initialized in memory")
		}

		const backupDir = "/var/lib/claviger/backups"

		err := system.PerformSecureBackup(db, backupDir, key)
		if err != nil {
			log.Printf("[Cron] ❌ Backup failed: %v", err)
			return err
		}

		log.Println("[Cron] ✅ Backup completed and encrypted successfully.")
		return nil
	})

	RegisterTask("update-check", "Update Checker", "Pings GitHub for new Claviger releases.", "🔄", "@every 12h", true, func() error {
		log.Println("[Cron] 🔄 Checking GitHub for system updates...")

		// 1. Check DB for current version, fallback to binary constant if missing
		currentVersion := storage.GetConfig(db, "current_version")
		if currentVersion == "" {
			currentVersion = system.CurrentVersion
		}

		// 2. Securely ping GitHub API
		hasUpdate, latestVersion, err := system.CheckGitHubForUpdates()
		if err != nil {
			log.Printf("[Cron] ❌ Update check failed: %v", err)
			return err
		}

		// 3. Update the database state for the Next.js UI
		if hasUpdate {
			log.Printf("[Cron] 🎉 New Update Available: %s (Current: %s)", latestVersion, currentVersion)

			// Signal the UI that an update is ready
			storage.SetConfig(db, "available_update_version", latestVersion)
		} else {
			log.Printf("[Cron] ✅ System is up to date (Running: %s)", currentVersion)

			// Clear any stale update flags to ensure the UI is clean
			storage.SetConfig(db, "available_update_version", "")
		}

		return nil
	})

	RegisterTask("zombie-cleanup", "Zombie Peer Cleanup", "Identifies and pauses VPN peers inactive for over 30 days.", "🧟", "@daily", true, func() error {
		count, err := network.PruneZombiePeers(db, 30)
		if err != nil {
			return err
		}
		if count > 0 {
			log.Printf("[Cron] 🧟 Cleanup complete. %d peers moved to paused state.", count)
		}
		return nil
	})

	engine.Start()
	log.Println("⏱️  Background Cron Engine started successfully.")
}

// RegisterTask adds a new task to the engine
func RegisterTask(id, name, desc, icon, schedule string, enabled bool, job func() error) {
	task := &Task{
		ID:          id,
		Name:        name,
		Description: desc,
		Icon:        icon,
		Schedule:    schedule,
		Enabled:     enabled,
		LastStatus:  "pending",
		job:         job,
	}
	Tasks[id] = task

	if enabled {
		EnableTask(id)
	}
}

// EnableTask activates a task in the cron schedule
func EnableTask(id string) error {
	task, exists := Tasks[id]
	if !exists {
		return fmt.Errorf("task not found")
	}

	// Wrapper function to record success/failure and timestamps
	wrappedJob := func() {
		task.LastRun = time.Now().Format("Jan 02, 15:04")
		err := task.job()
		if err != nil {
			task.LastStatus = "failed"
			log.Printf("[Cron Error] Task %s failed: %v", task.Name, err)
		} else {
			task.LastStatus = "success"
		}
	}

	entryID, err := engine.AddFunc(task.Schedule, wrappedJob)
	if err != nil {
		return err
	}

	task.entryID = entryID
	task.Enabled = true
	return nil
}

// DisableTask removes a task from the active schedule
func DisableTask(id string) {
	task, exists := Tasks[id]
	if exists && task.Enabled {
		engine.Remove(task.entryID)
		task.Enabled = false
		task.NextRun = "Disabled"
	}
}

// RunNow forces a task to execute immediately AND WAITS for the result
func RunNow(id string) error {
	task, exists := Tasks[id]
	if !exists {
		return fmt.Errorf("task not found")
	}

	// 🚀 FIX: We removed the 'go func()' so the HTTP request WAITS for this to finish
	task.LastRun = time.Now().Format("Jan 02, 15:04 (Manual)")

	// Execute the job and capture the error
	err := task.job()

	if err != nil {
		task.LastStatus = "failed"
		return err // Return the actual error so the HTTP handler knows it failed!
	}

	task.LastStatus = "success"
	return nil
}

// GetTasksForUI returns the active list with calculated "Next Run" times
func GetTasksForUI() []Task {
	var list []Task
	for _, t := range Tasks {
		if t.Enabled {
			entry := engine.Entry(t.entryID)
			t.NextRun = entry.Next.Format("Jan 02, 15:04")
		} else {
			t.NextRun = "Disabled"
		}
		list = append(list, *t)
	}
	return list
}

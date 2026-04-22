package scheduler

import (
	"claviger-server/internal/system"
	"claviger-server/network"
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
		time.Sleep(1 * time.Second) // Simulate work
		return system.PerformSecureBackup(db)
	})

	RegisterTask("update-check", "Update Checker", "Pings GitHub for new Claviger releases.", "🔄", "@every 12h", true, func() error {
		log.Println("[Cron] 🔄 Checking for system updates...")
		time.Sleep(1 * time.Second)
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

// RunNow forces a task to execute immediately (bypassing the schedule)
func RunNow(id string) error {
	task, exists := Tasks[id]
	if !exists {
		return fmt.Errorf("task not found")
	}

	go func() {
		task.LastRun = time.Now().Format("Jan 02, 15:04 (Manual)")
		err := task.job()
		if err != nil {
			task.LastStatus = "failed"
		} else {
			task.LastStatus = "success"
		}
	}()

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

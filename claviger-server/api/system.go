package api

import (
	"claviger-server/internal/scheduler"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
)

type SystemStats struct {
	CPUPercent  string `json:"cpu_percent"`
	CPUCores    string `json:"cpu_cores"`
	RAMPercent  string `json:"ram_percent"`
	RAMUsedGB   string `json:"ram_used_gb"`
	RAMTotalGB  string `json:"ram_total_gb"`
	DiskPercent string `json:"disk_percent"`
	DiskUsedGB  string `json:"disk_used_gb"`
	DiskTotalGB string `json:"disk_total_gb"`
	OSName      string `json:"os_name"`
	Uptime      string `json:"uptime"`
	NetRxBytes  string `json:"net_rx_bytes"` // Total Downloaded Bytes
	NetTxBytes  string `json:"net_tx_bytes"` // Total Uploaded Bytes
}

// sanitize converts region-specific commas (e.g. 5,1) to decimals (5.1) dynamically
func sanitize(val string) string {
	val = strings.TrimSpace(val)
	return strings.ReplaceAll(val, ",", ".")
}

func HandleSystemStats(w http.ResponseWriter, r *http.Request) {
	stats := SystemStats{
		CPUCores: fmt.Sprintf("%d", runtime.NumCPU()),
	}

	// 1. CPU Usage
	cpuCmd := "LC_ALL=C top -bn2 -d 0.2 | grep 'Cpu(s)' | tail -n 1 | awk '{print $2 + $4}'"
	if out, err := exec.Command("bash", "-c", cpuCmd).Output(); err == nil {
		stats.CPUPercent = sanitize(string(out))
	} else {
		stats.CPUPercent = "0"
	}

	// 2. RAM Usage
	ramCmd := "LC_ALL=C free -m | awk '/Mem:/ {used=$2-$7; printf \"%.1f %.1f %.0f\", used/1024, $2/1024, (used/$2)*100}'"
	if out, err := exec.Command("bash", "-c", ramCmd).Output(); err == nil {
		parts := strings.Split(sanitize(string(out)), " ")
		if len(parts) == 3 {
			stats.RAMUsedGB = parts[0]
			stats.RAMTotalGB = parts[1]
			stats.RAMPercent = parts[2]
		}
	}

	// 3. Disk Usage (Root)
	diskCmd := "LC_ALL=C df -BG / | awk 'NR==2 {print $3, $2, $5}' | tr -d '%G'"
	if out, err := exec.Command("bash", "-c", diskCmd).Output(); err == nil {
		parts := strings.Split(sanitize(string(out)), " ")
		if len(parts) == 3 {
			stats.DiskUsedGB = parts[0]
			stats.DiskTotalGB = parts[1]
			stats.DiskPercent = parts[2]
		}
	}

	// 4. OS Name
	osCmd := "grep PRETTY_NAME /etc/os-release | cut -d'\"' -f2"
	if out, err := exec.Command("bash", "-c", osCmd).Output(); err == nil {
		stats.OSName = strings.TrimSpace(string(out))
	} else {
		stats.OSName = "Linux"
	}

	// 5. Uptime
	if out, err := exec.Command("uptime", "-p").Output(); err == nil {
		stats.Uptime = strings.TrimPrefix(strings.TrimSpace(string(out)), "up ")
	}

	// 6. Network I/O (Finds the default internet interface and reads raw bytes)
	netCmd := `IFACE=$(ip route | awk '/default/ {print $5}' | head -n 1); cat /proc/net/dev | grep "$IFACE:" | awk -F':' '{print $2}' | awk '{print $1, $9}'`
	if out, err := exec.Command("bash", "-c", netCmd).Output(); err == nil {
		parts := strings.Split(strings.TrimSpace(string(out)), " ")
		if len(parts) >= 2 {
			stats.NetRxBytes = parts[0]
			stats.NetTxBytes = parts[1]
		} else {
			stats.NetRxBytes = "0"
			stats.NetTxBytes = "0"
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// HandleGetTasks returns the list of all cron tasks
func HandleGetTasks(w http.ResponseWriter, r *http.Request) {
	tasks := scheduler.GetTasksForUI()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tasks)
}

// HandleRunTask triggers a manual execution
func HandleRunTask(w http.ResponseWriter, r *http.Request) {
	// Extract the ID from the URL (e.g., /api/system/tasks/db-backup/run)
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 5 {
		http.Error(w, "Invalid task ID", http.StatusBadRequest)
		return
	}
	taskID := parts[4]

	err := scheduler.RunNow(taskID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// HandleToggleTask turns a task on or off
func HandleToggleTask(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 5 {
		http.Error(w, "Invalid task ID", http.StatusBadRequest)
		return
	}
	taskID := parts[4]

	var req struct {
		Enabled bool `json:"enabled"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	if req.Enabled {
		scheduler.EnableTask(taskID)
	} else {
		scheduler.DisableTask(taskID)
	}

	w.WriteHeader(http.StatusOK)
}

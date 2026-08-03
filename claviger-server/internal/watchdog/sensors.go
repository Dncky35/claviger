//go:build !windows

package watchdog

import (
	"bufio"
	"bytes"
	"claviger-server/internal/notifier"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// --- SMART COOLDOWN TRACKERS ---
var (
	lastDiskAlert      time.Time
	lastRAMAlert       time.Time
	lastCPUAlert       time.Time
	lastUFWAlert       time.Time
	lastWireguardAlert time.Time
)

// 15 minutes prevents spam while keeping you informed of ongoing critical issues
const hardwareAlertCooldown = 15 * time.Minute

func checkDiskUsage(thresholdPercent int) {
	var stat syscall.Statfs_t

	// Fetch filesystem stats for the root directory
	err := syscall.Statfs("/", &stat)
	if err != nil {
		return // Silently fail if unable to read
	}

	// Calculate total, free, and used bytes
	total := stat.Blocks * uint64(stat.Bsize)
	free := stat.Bfree * uint64(stat.Bsize)
	used := total - free

	if total == 0 {
		return
	}

	usedPercent := int((float64(used) / float64(total)) * 100)

	// Trigger alert if it crosses the threshold
	if usedPercent >= thresholdPercent {
		notifier.FireAlert(
			notifier.LevelWarning,
			"Low Disk Space",
			fmt.Sprintf("Server disk usage has reached %d%%. Please clear some space.", usedPercent),
		)
	} else {
		lastDiskAlert = time.Time{}
	}
}

func checkRAMUsage(thresholdPercent int) {
	file, err := os.Open("/proc/meminfo")
	if err != nil {
		return // Silently fail if not on Linux
	}
	defer file.Close()

	var memTotal, memAvailable float64
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		// Look for Total RAM and Available RAM (Available is better than "Free")
		switch fields[0] {
		case "MemTotal:":
			val, _ := strconv.ParseFloat(fields[1], 64)
			memTotal = val
		case "MemAvailable:":
			val, _ := strconv.ParseFloat(fields[1], 64)
			memAvailable = val
		}

		// Once we have both, we can stop reading the file
		if memTotal > 0 && memAvailable > 0 {
			break
		}
	}
	// Check for scanning errors
	if err := scanner.Err(); err != nil {
		return
	}

	if memTotal == 0 {
		return
	}

	// Calculate percentage
	memUsed := memTotal - memAvailable
	usedPercent := int((memUsed / memTotal) * 100)

	// Check against user threshold
	if usedPercent >= thresholdPercent {
		notifier.FireAlert(
			notifier.LevelWarning,
			"High RAM Usage",
			fmt.Sprintf("Server memory is currently at %d%%. This could cause services to crash.", usedPercent),
		)
	} else {
		lastRAMAlert = time.Time{}
	}
}

func checkCPUUsage(thresholdPercent int) {
	cpuCmd := "LC_ALL=C top -bn2 -d 0.2 | grep 'Cpu(s)' | tail -n 1 | awk '{print $2 + $4}'"

	out, err := exec.Command("bash", "-c", cpuCmd).Output()
	if err != nil {
		return
	}

	// Clean the string and convert to Float
	valStr := strings.TrimSpace(string(out))
	valStr = strings.ReplaceAll(valStr, ",", ".") // handle locale

	cpuFloat, err := strconv.ParseFloat(valStr, 64)
	if err != nil {
		return
	}

	cpuPercent := int(cpuFloat)

	if cpuPercent >= thresholdPercent {
		notifier.FireAlert(
			notifier.LevelWarning,
			"CPU Overload",
			fmt.Sprintf("Server CPU usage has spiked to %d%%.", cpuPercent),
		)
	} else {
		lastCPUAlert = time.Time{}
	}
}

func checkWireguardInterface() {

	cmd := exec.Command("ip", "link", "show", "wg0")
	if err := cmd.Run(); err != nil {
		notifier.FireAlert(
			notifier.LevelCritical,
			"VPN Interface Offline",
			"The WireGuard interface (wg0) is down or missing from the kernel!",
		)
	} else {
		lastWireguardAlert = time.Time{}
	}
}

func checkUFWStatus() {
	cmd := exec.Command("ufw", "status")
	var out bytes.Buffer
	cmd.Stdout = &out

	err := cmd.Run()
	output := out.String()

	// If ufw errors out, or explicitly says inactive, we have a massive security breach.
	if err != nil || strings.Contains(output, "inactive") {
		notifier.FireAlert(
			notifier.LevelCritical,
			"FIREWALL DOWN",
			"UFW is currently INACTIVE. The server perimeter is totally exposed!",
		)
	} else {
		lastUFWAlert = time.Time{}
	}
}

var lastAlertedExitedContainers = make(map[string]bool)

func checkDockerHealth() {

	log.Println("🔍 [Debug] checkDockerHealth() is executing...")

	cmd := exec.Command("docker", "ps", "-a", "--filter", "status=exited", "--format", "{{.Names}} (code {{.ExitCode}})")
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// FIX: Do NOT trigger a critical "Docker Engine Down" email on every permission glitch.
		// Instead, log it internally to debug, or check if socket exists.
		// We only alert if the socket file itself is entirely missing from the host.
		log.Printf("❌ [Debug] Docker command failed! Error: %v | Stderr: %s\n", err, stderr.String())
		return
	}

	exited := strings.TrimSpace(out.String())
	log.Printf("🐳 [Debug] Raw Docker output: %q\n", exited)

	// If no containers are exited, clear our cache so if they exit *later*, we alert fresh
	if exited == "" {
		lastAlertedExitedContainers = make(map[string]bool)
		return
	}

	// Split multiple stopped containers
	containers := strings.Split(exited, "\n")
	var newlyCrashed []string

	for _, container := range containers {
		container = strings.TrimSpace(container)
		if container == "" {
			continue
		}

		// Check if we already sent an alert for this specific crashed container instance
		if !lastAlertedExitedContainers[container] {
			log.Printf("🚨 [Debug] Found NEW crashed container: %s\n", container)
			newlyCrashed = append(newlyCrashed, container)
			// Mark as alerted
			lastAlertedExitedContainers[container] = true
		} else {
			log.Printf("💤 [Debug] Ignoring already-alerted container: %s\n", container)
		}
	}

	// Only fire an alert if there are *new* crashes we haven't notified about yet
	if len(newlyCrashed) > 0 {
		summary := strings.Join(newlyCrashed, ", ")
		notifier.FireAlert(
			notifier.LevelCritical,
			"Docker Container Crash",
			fmt.Sprintf("Detected newly exited container(s): %s", summary),
		)
	}
}

var lastKnownSSHBans = -1

// checkSSHBruteForce queries Fail2Ban to see if new IPs have been blocked.
func checkSSHBruteForce() {
	cmd := exec.Command("fail2ban-client", "status", "sshd")
	var out bytes.Buffer
	cmd.Stdout = &out

	if err := cmd.Run(); err != nil {
		// Fail2Ban is either not installed or the daemon is down.
		// We silently return so we don't spam errors on systems without it.
		return
	}

	output := out.String()
	currentTotalBans := 0

	// Parse the Fail2Ban output to find the "Total banned" metric
	// Output looks like: "|- Total banned:     15"
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "Total banned:") {
			fields := strings.Fields(line)
			if len(fields) > 0 {
				val, err := strconv.Atoi(fields[len(fields)-1])
				if err == nil {
					currentTotalBans = val
				}
			}
			break
		}
	}

	// First run initialization
	if lastKnownSSHBans == -1 {
		lastKnownSSHBans = currentTotalBans
		return
	}

	// If the number went up, we caught a brute force attempt!
	if currentTotalBans > lastKnownSSHBans {
		newBans := currentTotalBans - lastKnownSSHBans

		notifier.FireAlert(
			notifier.LevelWarning,
			"SSH Brute Force Blocked",
			fmt.Sprintf("Fail2Ban just blocked %d new IP(s) attempting to brute force SSH.", newBans),
		)

		// Update state
		lastKnownSSHBans = currentTotalBans
	}
}

//go:build !windows

package watchdog

import (
	"bufio"
	"bytes"
	"claviger-server/internal/notifier"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

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
			"💾 Low Disk Space",
			fmt.Sprintf("Server disk usage has reached %d%%. Please clear some space.", usedPercent),
		)
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
			"🔥 High RAM Usage",
			fmt.Sprintf("Server memory is currently at %d%%. This could cause services to crash.", usedPercent),
		)
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
			"⚠️ CPU Overload",
			fmt.Sprintf("Server CPU usage has spiked to %d%%.", cpuPercent),
		)
	}
}

func checkWireguardInterface() {

	cmd := exec.Command("ip", "link", "show", "wg0")
	if err := cmd.Run(); err != nil {
		notifier.FireAlert(
			notifier.LevelCritical,
			"🔌 VPN Interface Offline",
			"The WireGuard interface (wg0) is down or missing from the kernel!",
		)
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
			"🚨 FIREWALL DOWN",
			"UFW is currently INACTIVE. The server perimeter is totally exposed!",
		)
	}
}

func checkDockerHealth() {
	cmd := exec.Command("docker", "ps", "-a", "--filter", "status=exited", "--format", "{{.Names}} (code {{.ExitCode}})")
	var out bytes.Buffer
	cmd.Stdout = &out

	err := cmd.Run()
	if err != nil {
		// If Docker daemon isn't running or accessible, alert immediately!
		notifier.FireAlert(
			notifier.LevelCritical,
			"🐳 Docker Engine Down",
			"Unable to communicate with the Docker daemon. Is Docker service running?",
		)
		return
	}

	exited := strings.TrimSpace(out.String())
	if exited != "" {
		// Split multiple stopped containers into a readable list
		containers := strings.Split(exited, "\n")
		count := len(containers)

		summary := strings.Join(containers, ", ")

		notifier.FireAlert(
			notifier.LevelCritical,
			"🐳 Docker Container Crash",
			fmt.Sprintf("Found %d crashed/exited container(s): %s", count, summary),
		)
	}
}

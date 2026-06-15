package api

import (
	"bytes"
	"claviger-server/internal/firewall"
	"claviger-server/internal/security"
	"claviger-server/storage"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// Insight represents a security warning or suggestion
type Insight struct {
	Level   string `json:"level"` // "critical", "warning", "info"
	Title   string `json:"title"`
	Message string `json:"message"`
}

type FirewallRule struct {
	To     string `json:"to"`
	Action string `json:"action"`
	From   string `json:"from"`
}

type SecurityStats struct {
	UFWStatus string         `json:"ufw_status"`
	Rules     []FirewallRule `json:"rules"`
	Insights  []Insight      `json:"insights"`
}

// HandleSecurityStats parses UFW and returns the status, rules, and insights
func HandleSecurityStats(w http.ResponseWriter, r *http.Request) {
	stats := SecurityStats{
		UFWStatus: "inactive",
		Rules:     []FirewallRule{},
		Insights:  []Insight{},
	}

	hasPublicSSH := false
	hasPublicWeb := false
	hasPublicDB := false

	// 1. 🎯 NEW: Check if UFW is actually installed on the system
	if _, err := exec.LookPath("ufw"); err != nil {
		stats.UFWStatus = "not_installed"
	} else {
		// 2. Only try to get status if it is installed
		out, err := exec.Command("ufw", "status").Output()

		if err == nil {
			lines := strings.Split(string(out), "\n")
			parsingRules := false

			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}

				if strings.HasPrefix(line, "Status: active") {
					stats.UFWStatus = "active"
					continue
				}

				if strings.HasPrefix(line, "--") {
					parsingRules = true
					continue
				}

				if parsingRules {
					fields := strings.Fields(line)
					if len(fields) >= 3 {
						actionIdx := -1
						for i, v := range fields {
							if v == "ALLOW" || v == "DENY" || v == "REJECT" || v == "LIMIT" {
								actionIdx = i
								break
							}
						}

						if actionIdx > 0 && actionIdx < len(fields)-1 {
							toPort := strings.Join(fields[:actionIdx], " ")
							action := fields[actionIdx]
							fromIP := strings.Join(fields[actionIdx+1:], " ")

							stats.Rules = append(stats.Rules, FirewallRule{
								To:     toPort,
								Action: action,
								From:   fromIP,
							})

							// 🎯 INTERFACE FILTER CHECK
							// If the rule is explicitly locked down to a VPN or overlay interface,
							// it is safe! We will ignore it for the public vulnerability checks.
							toPortLower := strings.ToLower(toPort)
							isProtectedByInterface := strings.Contains(toPortLower, "on wg") ||
								strings.Contains(toPortLower, "on tailscale") ||
								strings.Contains(toPortLower, "on tun") ||
								strings.Contains(toPortLower, "on zt")

							// --- SMART SCANNER LOGIC ---
							if action == "ALLOW" && !isProtectedByInterface {
								isPublicOrigin := fromIP == "Anywhere" || fromIP == "Anywhere (v6)" || fromIP == "0.0.0.0/0" || fromIP == "::/0"

								if isPublicOrigin {
									// Check both default SSH (22) and your custom deployment port (2278)
									if strings.Contains(toPortLower, "22") || strings.Contains(toPortLower, "2278") {
										hasPublicSSH = true
									}
									if strings.Contains(toPortLower, "80") || strings.Contains(toPortLower, "443") {
										hasPublicWeb = true
									}
									if strings.Contains(toPortLower, "3306") || strings.Contains(toPortLower, "5432") || strings.Contains(toPortLower, "6379") {
										hasPublicDB = true
									}
								}
							}
						}
					}
				}
			}
		}
	}

	// --- GENERATE SMART INSIGHTS ---
	if stats.UFWStatus == "not_installed" {
		// 🎯 NEW: Insight specifically for missing UFW
		stats.Insights = append(stats.Insights, Insight{
			Level:   "critical",
			Title:   "Firewall Missing",
			Message: "UFW (Uncomplicated Firewall) is not installed on this server. Without a firewall, your server is completely exposed. Please install UFW via your terminal (e.g., 'apt install ufw') to secure your environment.",
		})
	} else if stats.UFWStatus != "active" {
		stats.Insights = append(stats.Insights, Insight{
			Level:   "critical",
			Title:   "Firewall Offline",
			Message: "Your firewall is installed but currently inactive. All ports are exposed to the internet. Enable UFW immediately.",
		})
	} else {
		if hasPublicSSH {
			stats.Insights = append(stats.Insights, Insight{
				Level:   "critical",
				Title:   "Public SSH Exposed",
				Message: "SSH access is wide open to the public internet. To prevent automated brute-force attacks, delete this public rule and restrict access exclusively to your Claviger/WireGuard interface.",
			})
		}
		if hasPublicWeb {
			stats.Insights = append(stats.Insights, Insight{
				Level:   "warning",
				Title:   "Public Web Traffic",
				Message: "Ports 80/443 are open to everywhere. If you are using a reverse proxy or CDN like Cloudflare, make sure to restrict these ports to only accept Cloudflare's edge IP addresses.",
			})
		}
		if hasPublicDB {
			stats.Insights = append(stats.Insights, Insight{
				Level:   "critical",
				Title:   "Database Exposed",
				Message: "A database port (MySQL, Postgres, or Redis) is exposed to the public internet. Restrict this to localhost or your internal VPN interfaces immediately.",
			})
		}

		// If they have a perfect, completely isolated configuration!
		if len(stats.Insights) == 0 {
			stats.Insights = append(stats.Insights, Insight{
				Level:   "info",
				Title:   "Zero Trust Aligned",
				Message: "Your firewall looks pristine. Administrative ports are isolated within your virtual overlay interfaces.",
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// HandleInstallUFW executes the package manager to install UFW on the host OS
func HandleInstallUFW(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	log.Println("📦 UI Triggered: Attempting to install UFW...")

	// 1. Run apt-get update first (best practice so it doesn't fail on old repo lists)
	updateCmd := exec.Command("apt-get", "update")
	_ = updateCmd.Run() // We ignore errors here, it's just a best-effort sync

	// 2. Run the actual installation command
	// Note: DEBIAN_FRONTEND=noninteractive stops APT from hanging on user prompts
	installCmd := exec.Command("apt-get", "install", "-y", "ufw")
	installCmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")

	output, err := installCmd.CombinedOutput()
	if err != nil {
		log.Printf("❌ Failed to install UFW: %v\nOutput: %s", err, string(output))
		http.Error(w, "Failed to install UFW", http.StatusInternalServerError)
		return
	}

	log.Println("✅ UFW successfully installed via Web UI.")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "success", "message": "UFW Installed"}`))
}

type SecurityActionReq struct {
	Action     string `json:"action"`      // "enable", "disable", "delete", "add"
	Port       string `json:"port"`        // e.g., "22", "80/tcp"
	RuleAction string `json:"rule_action"` // "ALLOW" or "DENY"
}

// HandleSecurityAction processes POST requests to modify the firewall safely.
func HandleSecurityAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req SecurityActionReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// --- SECURITY FIX: Validate Port Format & Prevent Command Injection ---
	// Validates pure numbers (22) or numbers with protocols (80/tcp, 51820/udp)
	matched, _ := regexp.MatchString(`^[0-9]+(/[a-z]+)?$`, req.Port)
	if (req.Action == "add" || req.Action == "delete") && !matched {
		http.Error(w, "Invalid port format rejected for security reasons", http.StatusBadRequest)
		return
	}

	var err error

	switch req.Action {
	case "enable":
		log.Println("🛡️ Enabling UFW with isolated Zero-Trust baseline rules...")

		db := storage.InitDB()
		defer db.Close()

		// --- THE FIX: Ensure Setup Was Completed ---
		if storage.GetConfig(db, "node_id") == "" {
			log.Fatal("❌ Node is not configured! Please run 'sudo claviger-server setup' first.")
		}

		wgPort := storage.GetConfig(db, "wg_port")

		if wgPort == "" {
			wgPort = "51820"
		}

		if err := firewall.SetupFirewall(wgPort); err != nil {
			log.Printf("❌ Failed to set up firewall: %v", err)
			http.Error(w, fmt.Sprintf("Failed to set up firewall: %v", err), http.StatusInternalServerError)
			return
		}

		// 3. Force enable the firewall
		err = runUfwCmd("--force", "enable")

	case "disable":
		log.Println("🧹 Disabling UFW and dropping baseline rules...")

		db := storage.InitDB()
		defer db.Close()

		// --- THE FIX: Ensure Setup Was Completed ---
		if storage.GetConfig(db, "node_id") == "" {
			log.Fatal("❌ Node is not configured! Please run 'sudo claviger-server setup' first.")
		}

		wgPort := storage.GetConfig(db, "wg_port")

		if wgPort == "" {
			wgPort = "51820"
		}

		if err := firewall.TeardownFirewall(wgPort); err != nil {
			log.Printf("❌ Failed to tear down firewall: %v", err)
			http.Error(w, fmt.Sprintf("Failed to tear down firewall: %v", err), http.StatusInternalServerError)
			return
		}

		exec.Command("ufw", "allow", "22/tcp").Run() // Ensure SSH isn't locked out when firewall is disabled

		err = runUfwCmd("disable")

	case "add":
		log.Printf("➕ Adding custom firewall rule for port: %s", req.Port)
		err = runUfwCmd("allow", req.Port)

	case "delete":
		log.Printf("❌ Removing custom firewall rule for port: %s", req.Port)
		actionLower := "allow"
		if strings.ToLower(req.RuleAction) == "deny" {
			actionLower = "deny"
		}
		err = runUfwCmd("delete", actionLower, req.Port)

	default:
		http.Error(w, "Unknown security action", http.StatusBadRequest)
		return
	}

	// Sync changes into the active kernel
	if err == nil {
		runUfwCmd("reload")
	}

	// Response formatting
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		log.Printf("❌ Firewall action failed: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "error",
			"message": fmt.Sprintf("Firewall execution failed: %v", err),
		})
		return
	}

	log.Printf("✅ Firewall action [%s] executed successfully", req.Action)
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

// 🎯 HELPER: Executes commands while capturing standard error pipelines for clean logging
func runUfwCmd(args ...string) error {
	cmd := exec.Command("ufw", args...)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		// Returns the actual error string from UFW (e.g., "Rule already exists")
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func HandleCloudflareLockdown(w http.ResponseWriter, r *http.Request) {
	db := storage.InitDB()
	defer db.Close()

	proxyProvider := storage.GetConfig(db, "proxy_provider")
	if proxyProvider != "cloudflare" {
		http.Error(w, "Cloudflare lockdown is only applicable if Cloudflare is set as the proxy provider.", http.StatusBadRequest)
		return
	}

	// 1. Fetch live IPs
	ips, err := security.FetchCloudflareIPs()
	if err != nil {
		http.Error(w, "Failed to reach Cloudflare API", http.StatusBadGateway)
		return
	}

	// 2. Apply UFW Rules (NGINX LOGIC REMOVED FROM HERE)
	if err := security.LockdownUFW(ips); err != nil {
		http.Error(w, "Failed to apply firewall rules", http.StatusInternalServerError)
		return
	}

	// 3. Store the active IPs to SQLite for the Revert function
	ipString := strings.Join(ips, ",")
	storage.SetConfig(db, "cloudflare_active_ips", ipString)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "success",
		"message": "Ports 80/443 are now locked down to Cloudflare Edge Networks.",
	})
}

func HandleEnableStandardRules(w http.ResponseWriter, r *http.Request) {
	if err := security.EnableStandardRules(); err != nil {
		http.Error(w, "Failed to enabling rules", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "success",
		"message": "Firewall enabled for http/https connections."})
}

func HandleDisableStandardRules(w http.ResponseWriter, r *http.Request) {
	if err := security.DisableStandardRules(); err != nil {
		http.Error(w, "Failed to disabling rules", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "success",
		"message": "Firewall disabled for http/https connections."})
}

func HandleCloudflareRevert(w http.ResponseWriter, r *http.Request) {
	db := storage.InitDB()
	defer db.Close()

	// 1. Read the exact IPs that were applied from the database
	ipString := storage.GetConfig(db, "cloudflare_active_ips")
	var activeIPs []string
	if ipString != "" {
		activeIPs = strings.Split(ipString, ",")
	}

	// 2. Revert the firewall using ONLY those IPs
	if err := security.RevertLockdown(activeIPs); err != nil {
		http.Error(w, "Failed to revert lockdown", http.StatusInternalServerError)
		return
	}

	// 3. Clear the database record now that they are removed
	storage.SetConfig(db, "cloudflare_active_ips", "")

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "success",
		"message": "Firewall restored to open mode."})
}

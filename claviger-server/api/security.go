package api

import (
	"encoding/json"
	"net/http"
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

	out, err := exec.Command("ufw", "status").Output()

	hasSSH := false
	hasWeb := false
	hasDB := false

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

						// --- SCANNER LOGIC ---
						if action == "ALLOW" && (fromIP == "Anywhere" || fromIP == "Anywhere (v6)" || fromIP == "0.0.0.0/0") {
							if strings.Contains(toPort, "22") {
								hasSSH = true
							}
							if strings.Contains(toPort, "80") || strings.Contains(toPort, "443") {
								hasWeb = true
							}
							if strings.Contains(toPort, "3306") || strings.Contains(toPort, "5432") || strings.Contains(toPort, "6379") {
								hasDB = true
							}
						}
					}
				}
			}
		}
	}

	// --- GENERATE INSIGHTS ---
	if stats.UFWStatus != "active" {
		stats.Insights = append(stats.Insights, Insight{
			Level:   "critical",
			Title:   "Firewall Offline",
			Message: "Your firewall is inactive. All ports are currently exposed to the internet. Enable UFW immediately.",
		})
	} else {
		if hasSSH {
			stats.Insights = append(stats.Insights, Insight{
				Level:   "critical",
				Title:   "Public SSH Exposed",
				Message: "Port 22 is open to the world. To prevent bot brute-force attacks, delete this rule and use your Claviger VPN subnet (10.8.0.0/24) to connect via SSH.",
			})
		}
		if hasWeb {
			stats.Insights = append(stats.Insights, Insight{
				Level:   "warning",
				Title:   "Public Web Traffic",
				Message: "Ports 80/443 are open to everywhere. Consider using a Reverse Proxy or CDN (like Cloudflare, Fastly, or AWS) and restricting these ports to only allow traffic from their specific IP ranges.",
			})
		}
		if hasDB {
			stats.Insights = append(stats.Insights, Insight{
				Level:   "critical",
				Title:   "Database Exposed",
				Message: "A database port (MySQL, Postgres, or Redis) is exposed to the public internet. Restrict this to localhost or your VPN subnet immediately.",
			})
		}

		// If they have a perfect setup!
		if len(stats.Insights) == 0 {
			stats.Insights = append(stats.Insights, Insight{
				Level:   "info",
				Title:   "Zero Trust Aligned",
				Message: "Your firewall rules look solid. No obvious external misconfigurations detected.",
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

type SecurityActionReq struct {
	Action     string `json:"action"`      // "enable", "disable", "delete", "add"
	Port       string `json:"port"`        // e.g., "22", "80/tcp"
	RuleAction string `json:"rule_action"` // "ALLOW" or "DENY"
}

// HandleSecurityAction processes POST requests to modify the firewall
func HandleSecurityAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req SecurityActionReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// --- SECURITY FIX: Block Command Injection ---
	// Only allow numbers, or numbers followed by /tcp or /udp
	matched, _ := regexp.MatchString(`^[0-9]+(/[a-z]+)?$`, req.Port)
	if (req.Action == "add" || req.Action == "delete") && !matched {
		http.Error(w, "Invalid port format rejected for security reasons", http.StatusBadRequest)
		return
	}

	var err error

	switch req.Action {
	case "enable":
		// Automatically allow standard required ports
		exec.Command("ufw", "allow", "22/tcp").Run()
		exec.Command("ufw", "allow", "51820/udp").Run()

		// THE FIX: Explicitly trust all internal VPN traffic so the Hub never gets blocked!
		exec.Command("ufw", "allow", "in", "on", "wg0").Run()

		// Finally, force enable UFW
		err = exec.Command("ufw", "--force", "enable").Run()

	case "disable":
		err = exec.Command("ufw", "disable").Run()

	case "add":
		// e.g., ufw allow 80
		err = exec.Command("ufw", "allow", req.Port).Run()

	case "delete":
		// e.g., ufw delete allow 22
		actionLower := strings.ToLower(req.RuleAction)
		err = exec.Command("ufw", "delete", actionLower, req.Port).Run()
	}

	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": err.Error()})
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

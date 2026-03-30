package firewall

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
)

// RoleConfig holds all the granular permissions for a single user
type RoleConfig struct {
	ClientIP      string
	HubIP         string // e.g., "10.8.0.1"
	HubPort       string // e.g., "18080"
	AllowInternet bool
	AllowIntranet bool
	AllowHub      bool
	AllowedIPs    string
	AllowedPorts  string
}

// ApplyRoleRules translates UI checkboxes into strict Linux Kernel rules.
func ApplyRoleRules(config RoleConfig) error {
	// 1. Clean up any ghost rules so we start fresh
	RemoveRoleRules(config.ClientIP)

	log.Printf("🛡️ Enforcing Zero Trust rules for %s", config.ClientIP)

	// ==========================================
	// STEP 1: THE SAFETY NET
	// Insert DROP rules FIRST. As we add ACCEPT rules below,
	// they will stack on top of these, pushing the DROP rules to the very bottom!
	// ==========================================
	exec.Command("iptables", "-I", "INPUT", "-i", "wg0", "-s", config.ClientIP, "-j", "DROP").Run()
	exec.Command("iptables", "-I", "FORWARD", "-i", "wg0", "-s", config.ClientIP, "-j", "DROP").Run()

	// ==========================================
	// STEP 2: CRITICAL SYSTEM ACCESS
	// Always allow DNS (Port 53) to the Hub, otherwise the internet won't resolve!
	// ==========================================
	exec.Command("iptables", "-I", "INPUT", "-i", "wg0", "-s", config.ClientIP, "-d", config.HubIP, "-p", "udp", "--dport", "53", "-j", "ACCEPT").Run()

	// ==========================================
	// STEP 3: HUB ACCESS (Checkbox)
	// ==========================================
	if config.AllowHub {
		exec.Command("iptables", "-I", "INPUT", "-i", "wg0", "-s", config.ClientIP, "-d", config.HubIP, "-p", "tcp", "--dport", config.HubPort, "-j", "ACCEPT").Run()
	}

	// --- Prepare Granular Filters ---
	ports := strings.ReplaceAll(config.AllowedPorts, " ", "")
	ips := strings.ReplaceAll(config.AllowedIPs, " ", "")

	hasPortsFilter := ports != "ALL" && ports != ""
	hasIPsFilter := ips != "ALL" && ips != ""

	targetIPs := []string{"0.0.0.0/0"} // Default to everywhere
	if hasIPsFilter {
		targetIPs = strings.Split(ips, ",")
	}

	portArgs := []string{}
	if hasPortsFilter {
		// Apply port filters to TCP (covers 95% of web/app traffic)
		portArgs = []string{"-p", "tcp", "-m", "multiport", "--dports", ports}
	}

	// ==========================================
	// STEP 4: INTRANET ACCESS (Checkbox)
	// Can they talk to 10.8.0.x devices?
	// ==========================================
	if config.AllowIntranet {
		for _, ip := range targetIPs {
			target := ip
			if !hasIPsFilter {
				target = "10.8.0.0/24" // Limit "ALL" strictly to the local subnet
			}

			// Forwarding to other peers
			fwdArgs := []string{"-I", "FORWARD", "-i", "wg0", "-s", config.ClientIP, "-d", target}
			fwdArgs = append(fwdArgs, portArgs...)
			fwdArgs = append(fwdArgs, "-j", "ACCEPT")
			exec.Command("iptables", fwdArgs...).Run()

			// Accessing the Server itself
			if target == "10.8.0.0/24" || target == config.HubIP {
				inArgs := []string{"-I", "INPUT", "-i", "wg0", "-s", config.ClientIP, "-d", config.HubIP}
				inArgs = append(inArgs, portArgs...)
				inArgs = append(inArgs, "-j", "ACCEPT")
				exec.Command("iptables", inArgs...).Run()
			}
		}
	}

	// ==========================================
	// STEP 5: GLOBAL INTERNET (Checkbox)
	// Can they browse outside the VPN?
	// ==========================================
	if config.AllowInternet {
		for _, ip := range targetIPs {
			fwdArgs := []string{"-I", "FORWARD", "-i", "wg0", "-s", config.ClientIP}

			if !hasIPsFilter {
				// If no specific IP, allow everywhere EXCEPT the local intranet
				// (because Intranet is handled by the toggle above)
				fwdArgs = append(fwdArgs, "!", "-d", "10.8.0.0/24")
			} else {
				fwdArgs = append(fwdArgs, "-d", ip)
			}

			fwdArgs = append(fwdArgs, portArgs...)
			fwdArgs = append(fwdArgs, "-j", "ACCEPT")
			exec.Command("iptables", fwdArgs...).Run()
		}
	}

	return nil
}

// RemoveRoleRules wipes a user's specific rules from the kernel.
func RemoveRoleRules(clientIP string) {
	chains := []string{"INPUT", "FORWARD"}

	for _, chain := range chains {
		for {
			// CRITICAL FIX: We use 'grep -w' so IP 10.8.0.2 doesn't accidentally delete rules for 10.8.0.25!
			cmd := exec.Command("sh", "-c", fmt.Sprintf("iptables -L %s -n --line-numbers | grep -w '%s' | awk '{print $1}' | head -n 1", chain, clientIP))
			out, err := cmd.Output()
			ruleNum := strings.TrimSpace(string(out))

			if err != nil || ruleNum == "" {
				break
			}
			exec.Command("iptables", "-D", chain, ruleNum).Run()
		}
	}
}

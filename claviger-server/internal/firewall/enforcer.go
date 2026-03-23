package firewall

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
)

// ApplyRoleRules translates a role string (like "80,443" or "ALL") into strict iptables rules.
func ApplyRoleRules(clientIP string, allowedPorts string) error {
	// 1. Clean up any existing rules for this IP to prevent duplicates
	RemoveRoleRules(clientIP)

	log.Printf("🛡️ Enforcing Zero Trust rules for %s (Allowed: %s)", clientIP, allowedPorts)

	allowedPorts = strings.TrimSpace(strings.ToUpper(allowedPorts))

	// If the user is an Admin, we allow them to talk to everything on the server
	if allowedPorts == "ALL" {
		exec.Command("iptables", "-I", "INPUT", "-i", "wg0", "-s", clientIP, "-j", "ACCEPT").Run()
		exec.Command("iptables", "-I", "FORWARD", "-i", "wg0", "-s", clientIP, "-j", "ACCEPT").Run()
		return nil
	}

	// For specific ports (e.g., "80,443" or "22,5432")
	// We allow those specific TCP ports...
	ports := strings.ReplaceAll(allowedPorts, " ", "")

	// Allow to server itself (INPUT)
	exec.Command("iptables", "-I", "INPUT", "-i", "wg0", "-s", clientIP, "-p", "tcp", "-m", "multiport", "--dports", ports, "-j", "ACCEPT").Run()
	// Allow to Docker containers or other peers (FORWARD)
	exec.Command("iptables", "-I", "FORWARD", "-i", "wg0", "-s", clientIP, "-p", "tcp", "-m", "multiport", "--dports", ports, "-j", "ACCEPT").Run()

	// ...and then we STRICTLY DROP everything else from this specific user.
	// (Since we use -I to Insert at the top, we append the DROP rule immediately after,
	// which effectively puts the DROP rule BELOW the ACCEPT rules).
	exec.Command("iptables", "-A", "INPUT", "-i", "wg0", "-s", clientIP, "-j", "DROP").Run()
	exec.Command("iptables", "-A", "FORWARD", "-i", "wg0", "-s", clientIP, "-j", "DROP").Run()

	return nil
}

// RemoveRoleRules wipes a user's specific micro-segmentation rules from the kernel.
// We run this when a user is revoked or disconnected.
func RemoveRoleRules(clientIP string) {
	// iptables doesn't have a simple "delete all by IP" command,
	// so we loop through and delete rules matching this IP until none are left.
	chains := []string{"INPUT", "FORWARD"}

	for _, chain := range chains {
		for {
			// Find the rule number for this IP
			cmd := exec.Command("sh", "-c", fmt.Sprintf("iptables -L %s -n --line-numbers | grep %s | awk '{print $1}' | head -n 1", chain, clientIP))
			out, err := cmd.Output()
			ruleNum := strings.TrimSpace(string(out))

			// If no more rules exist for this IP, break the loop
			if err != nil || ruleNum == "" {
				break
			}

			// Delete the rule
			exec.Command("iptables", "-D", chain, ruleNum).Run()
		}
	}
}

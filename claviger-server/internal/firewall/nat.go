package firewall

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
)

func getPublicInterface() (string, error) {
	cmd := exec.Command("sh", "-c", "ip route | grep default | awk '{print $5}' | head -n 1")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("could not determine default interface: %v", err)
	}
	iface := strings.TrimSpace(string(output))
	if iface == "" {
		return "", fmt.Errorf("default interface is empty")
	}
	return iface, nil
}

// EnableInternet allows WireGuard clients to route traffic to the public internet
func EnableInternet() error {
	iface, err := getPublicInterface()
	if err != nil {
		log.Printf("⚠️ Warning: Could not find public interface. Defaulting to eth0. Error: %v", err)
		iface = "eth0"
	}

	log.Printf("🌐 Enabling Global Internet Routing on interface %s...", iface)

	// Safely remove the rule first to prevent duplicate entries
	exec.Command("iptables", "-t", "nat", "-D", "POSTROUTING", "-s", "10.8.0.0/24", "-o", iface, "-j", "MASQUERADE").Run()

	// Add the MASQUERADE rule
	err = exec.Command("iptables", "-t", "nat", "-A", "POSTROUTING", "-s", "10.8.0.0/24", "-o", iface, "-j", "MASQUERADE").Run()
	if err != nil {
		return fmt.Errorf("failed to enable internet routing: %v", err)
	}

	return nil
}

// DisableInternet blocks WireGuard clients from reaching the public internet
func DisableInternet() error {
	iface, err := getPublicInterface()
	if err != nil {
		iface = "eth0"
	}

	log.Printf("🛑 Disabling Global Internet Routing on interface %s...", iface)

	// Delete the MASQUERADE rule
	err = exec.Command("iptables", "-t", "nat", "-D", "POSTROUTING", "-s", "10.8.0.0/24", "-o", iface, "-j", "MASQUERADE").Run()
	if err != nil {
		log.Printf("   [i] Internet routing is already disabled.")
	}

	return nil
}

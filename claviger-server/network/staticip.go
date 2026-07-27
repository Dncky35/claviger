package network

import (
	"fmt"
	"log"
	"os"
	"os/exec"
)

// ApplyStaticIP auto-detects the host network manager and sets the static IP.
func ApplyStaticIP(iface, ipCidr, gateway string) error {

	// 1. Check for Netplan (Ubuntu Standard)
	if _, err := exec.LookPath("netplan"); err == nil {
		log.Println("🛠️ [Network] Netplan detected. Applying static IP...")
		return applyViaNetplan(iface, ipCidr, gateway)
	}

	// 2. Check for NetworkManager (Debian / Fedora Standard)
	if _, err := exec.LookPath("nmcli"); err == nil {
		log.Println("🛠️ [Network] NetworkManager detected. Applying static IP...")
		return applyViaNmcli(iface, ipCidr, gateway)
	}

	return fmt.Errorf("neither netplan nor nmcli found on this system")
}

// ---------------------------------------------------------
// UBUNTU: NETPLAN IMPLEMENTATION
// ---------------------------------------------------------
func applyViaNetplan(iface, ipCidr, gateway string) error {
	// Generate the Netplan YAML configuration
	yamlConfig := fmt.Sprintf(`network:
  version: 2
  renderer: networkd
  ethernets:
    %s:
      dhcp4: no
      addresses:
        - %s
      routes:
        - to: default
          via: %s
      nameservers:
        addresses: [8.8.8.8, 1.1.1.1]
`, iface, ipCidr, gateway)

	// Write it to a Claviger-specific file so it overrides default DHCP
	configPath := "/etc/netplan/99-claviger-static.yaml"
	if err := os.WriteFile(configPath, []byte(yamlConfig), 0644); err != nil {
		return fmt.Errorf("failed to write netplan config: %v", err)
	}

	// Apply the changes
	cmd := exec.Command("netplan", "apply")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("netplan apply failed: %s", string(output))
	}

	return nil
}

// ---------------------------------------------------------
// DEBIAN: NETWORKMANAGER IMPLEMENTATION
// ---------------------------------------------------------
func applyViaNmcli(iface, ipCidr, gateway string) error {
	// Set the interface to manual and apply IPs
	cmdMod := exec.Command("nmcli", "con", "mod", iface,
		"ipv4.addresses", ipCidr,
		"ipv4.gateway", gateway,
		"ipv4.dns", "8.8.8.8,1.1.1.1",
		"ipv4.method", "manual",
	)

	if output, err := cmdMod.CombinedOutput(); err != nil {
		return fmt.Errorf("nmcli mod failed: %s", string(output))
	}

	// Bounce the interface to apply (ignore errors as the connection might drop)
	exec.Command("nmcli", "con", "up", iface).Run()

	return nil
}

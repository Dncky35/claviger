package cli

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"time"
)

// HandleAutostart toggles the background daemon's boot-time auto-connect feature
func HandleAutoConnect(action string) {
	var enable bool
	switch action {
	case "enable":
		enable = true
	case "disable":
		enable = false
	default:
		log.Fatalf("❌ Invalid action: '%s'. Usage: claviger-client autostart <enable|disable>", action)
	}

	fmt.Println("⏳ Sending auto-connect preference to Daemon...")

	// 1. Connect to the Daemon via local IPC
	conn, err := net.DialTimeout("tcp", "127.0.0.1:42899", 2*time.Second)
	if err != nil {
		log.Fatalf("❌ Daemon not reachable: %v\nPlease ensure the Claviger Background Service is running.", err)
	}
	defer conn.Close()

	// 2. Send the SET_AUTOCONNECT command with the \n delimiter
	payload := fmt.Sprintf("SET_AUTOCONNECT|%t\n", enable)
	if _, err := conn.Write([]byte(payload)); err != nil {
		log.Fatalf("❌ Failed to send command to Daemon: %v", err)
	}

	// 3. Wait for the Daemon's response
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	resp, err := bufio.NewReader(conn).ReadString('\n')

	if err != nil && err != io.EOF {
		log.Fatalf("❌ Failed to read response from Daemon: %v", err)
	}

	resp = strings.TrimSpace(resp)

	// 4. Handle success or failure
	if resp == "OK" {
		if enable {
			fmt.Println("🔄 Auto-Connect ENABLED.")
			fmt.Println("The background daemon will now automatically connect your active profile on system boot.")
		} else {
			fmt.Println("🛑 Auto-Connect DISABLED.")
			fmt.Println("Claviger will wait for a manual connection command after reboot.")
		}
	} else {
		log.Fatalf("❌ Daemon failed to update auto-connect preference: %s\n(Check daemon logs for details)", resp)
	}
}

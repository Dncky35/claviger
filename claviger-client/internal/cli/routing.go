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

func HandleGlobalRouting(action string) {
	var enable bool
	switch action {
	case "enable":
		enable = true
	case "disable":
		enable = false
	default:
		log.Fatalf("❌ Invalid action: '%s'. Usage: claviger-client global <enable|disable>", action)
	}

	fmt.Println("⏳ Sending routing preference to Daemon...")

	// 1. Connect to the Daemon via local IPC
	conn, err := net.DialTimeout("tcp", "127.0.0.1:42899", 2*time.Second)
	if err != nil {
		log.Fatalf("❌ Daemon not reachable: %v\nPlease ensure the Claviger Background Service is running.", err)
	}
	defer conn.Close()

	// 2. Send the SET_GLOBAL_ROUTING command with the \n delimiter
	payload := fmt.Sprintf("SET_GLOBAL_ROUTING|%t\n", enable)
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
			fmt.Println("🌐 Global Routing ENABLED successfully.")
		} else {
			fmt.Println("🌗 Global Routing DISABLED successfully.")
		}
	} else {
		log.Fatalf("❌ Daemon failed to update routing preference: %s\n(Check daemon logs for details)", resp)
	}
}

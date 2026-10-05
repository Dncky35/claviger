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

func HandleRemove(profileID string) {
	profileID = strings.TrimSpace(profileID)
	if profileID == "" {
		log.Fatalf("❌ Error: Profile ID cannot be empty.")
	}

	fmt.Println("⏳ Requesting Daemon to remove server profile...")

	// 1. Connect to the Daemon via local IPC
	conn, err := net.DialTimeout("tcp", "127.0.0.1:42899", 2*time.Second)
	if err != nil {
		log.Fatalf("❌ Daemon not reachable: %v\nPlease ensure the Claviger Background Service is running.", err)
	}
	defer conn.Close()

	// 2. Send the REMOVE command (Crucial: Include the \n)
	payload := fmt.Sprintf("REMOVE|%s\n", profileID)
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

	// 4. Handle success or failure based on the Daemon's ruling
	if resp == "OK" {
		fmt.Println("✅ Server deleted successfully.")
	} else {
		log.Fatalf("❌ Daemon failed to remove the profile: %s\n(Check daemon logs for details)", resp)
	}
}

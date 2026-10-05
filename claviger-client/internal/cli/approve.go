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

func HandleApprove(tokenString string) {
	fmt.Println("⏳ Sending Visa to Daemon for validation...")

	tokenString = strings.TrimSpace(tokenString)
	if tokenString == "" {
		log.Fatalf("❌ Error: Visa token cannot be empty.")
	}

	// 1. Connect to the Daemon via local IPC
	conn, err := net.DialTimeout("tcp", "127.0.0.1:42899", 2*time.Second)
	if err != nil {
		log.Fatalf("❌ Daemon not reachable: %v\nPlease ensure the Claviger Background Service is running.", err)
	}
	defer conn.Close()

	// 2. Send the APPROVE command (Crucial: Include the \n)
	payload := fmt.Sprintf("APPROVE|%s\n", tokenString)
	if _, err := conn.Write([]byte(payload)); err != nil {
		log.Fatalf("❌ Failed to send command to Daemon: %v", err)
	}

	// 3. Wait for the Daemon's response
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	resp, err := bufio.NewReader(conn).ReadString('\n')

	// If the daemon closes the connection right after writing "OK" without a newline,
	// we will get io.EOF. We only want to fail on true network errors.
	if err != nil && err != io.EOF {
		log.Fatalf("❌ Failed to read response from Daemon: %v", err)
	}

	resp = strings.TrimSpace(resp)

	// 4. Handle success or failure based on the Daemon's ruling
	if resp == "OK" {
		fmt.Println("✅ Visa Accepted! You are now successfully enrolled.")
		fmt.Println("Run 'claviger connect' to establish the tunnel.")
	} else {
		// If it's not OK, the Daemon responds with "ER" or a specific error message
		log.Fatalf("❌ Daemon rejected the Visa: %s\n(Check daemon logs for details)", resp)
	}
}

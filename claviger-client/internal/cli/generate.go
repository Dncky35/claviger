package cli

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"strings"
	"time"
)

func HandleGenerate() {
	fmt.Println("⏳ Requesting cryptographic keys from the Daemon...")

	// 1. Connect to the Daemon via local IPC
	conn, err := net.DialTimeout("tcp", "127.0.0.1:42899", 2*time.Second)
	if err != nil {
		log.Fatalf("❌ Daemon not reachable: %v\nPlease ensure the Claviger Daemon is running (e.g., systemctl start claviger).", err)
	}
	defer conn.Close()

	// 2. Ask the Daemon to generate the profile, save it to disk, and return the token
	if _, err := conn.Write([]byte("GENERATE_TOKEN\n")); err != nil {
		log.Fatalf("❌ Failed to send command to Daemon: %v", err)
	}

	// 3. Wait for the Daemon's response
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	resp, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		log.Fatalf("❌ Failed to read response from Daemon: %v", err)
	}

	resp = strings.TrimSpace(resp)
	parts := strings.Split(resp, "|")

	// 4. Handle success or failure
	if len(parts) == 2 && parts[0] == "OK" {
		token := parts[1]
		fmt.Println("\n✅ PASSPORT GENERATED SUCCESSFULLY")
		fmt.Println("Send this token to your Network Administrator:")
		fmt.Println("---------------------------------------------------")
		fmt.Println(token)
		fmt.Println("---------------------------------------------------")
	} else {
		log.Fatalf("❌ Daemon failed to generate token: %s", resp)
	}
}

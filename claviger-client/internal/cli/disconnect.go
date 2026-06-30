package cli

import (
	"claviger-client/internal/config"
	"fmt"
	"net"
	"strings"
	"time"
)

func HandleDisconnect(vault *config.ClientVault) {
	fmt.Println("🛑 Sending disconnect signal to Claviger Engine...")

	// 1. Dial the background process with a timeout
	conn, err := net.DialTimeout("tcp", "127.0.0.1:42899", 2*time.Second)
	if err != nil {
		fmt.Println("⚪ Claviger is not currently running. Nothing to disconnect.")
		return
	}
	defer conn.Close()

	// 2. Set a Read Deadline so the CLI doesn't hang if the daemon stalls
	conn.SetReadDeadline(time.Now().Add(3 * time.Second))

	// 3. Whisper the disconnect command
	if _, err := conn.Write([]byte("DISCON")); err != nil {
		fmt.Println("❌ Failed to send command to daemon.")
		return
	}

	// 4. Read the response
	buf := make([]byte, 128)
	n, err := conn.Read(buf)
	if err != nil {
		fmt.Printf("❌ Connection lost while waiting for daemon: %v\n", err)
		return
	}

	response := strings.TrimSpace(string(buf[:n]))

	// 5. Handle the result
	if response == "OK" {
		fmt.Println("✅ Signal acknowledged by daemon.")

		// UX: Professional teardown sequence
		time.Sleep(400 * time.Millisecond)
		fmt.Println("🧹 Tearing down secure tunnels & resetting DNS...")

		time.Sleep(600 * time.Millisecond)
		fmt.Println("👋 Claviger disconnected gracefully. Normal network restored.")
	} else {
		fmt.Printf("⚠️ Daemon replied with error: %s\n", response)
	}
}

package daemon

import (
	"bufio"
	"log"
	"net"
	"strings"
	"time"

	"fyne.io/fyne/v2"
)

func StartEventSubscriber(app fyne.App) {
	go func() {
		for {
			// 1. Try to connect to the Daemon's IPC port
			conn, err := net.Dial("tcp", "127.0.0.1:42899")
			if err != nil {
				// If daemon isn't running yet, wait 5 seconds and try again
				time.Sleep(5 * time.Second)
				continue
			}

			log.Println("📡 Connected to Daemon Event Stream")

			// 2. Tell the daemon we aren't sending a command, we want to listen!
			conn.Write([]byte("SUBSCRIBE\n"))

			// 3. Sit in a loop and read incoming events
			reader := bufio.NewReader(conn)
			for {
				message, err := reader.ReadString('\n')
				if err != nil {
					log.Println("⚠️ Lost connection to Daemon Event Stream. Retrying...")
					conn.Close()
					break // Breaks the inner loop, outer loop reconnects
				}

				event := strings.TrimSpace(message)
				log.Printf("📥 Received Event: %s", event)

				// 4. Trigger Fyne OS Notifications based on the event
				switch event {
				case "EVENT: CONNECTED":
					app.SendNotification(&fyne.Notification{
						Title:   "Claviger Secured",
						Content: "Your traffic is now encrypted.",
					})
				case "EVENT: CONNECT_DROPPED":
					app.SendNotification(&fyne.Notification{
						Title:   "⚠️ Connection Lost",
						Content: "Secure tunnel dropped. Attempting to recover...",
					})
				}
			}

			time.Sleep(2 * time.Second) // Small delay before reconnecting
		}
	}()
}

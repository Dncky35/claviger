package daemon

import (
	"fmt"
	"net"
	"sync"
)

var (
	subscribers map[net.Conn]bool
	subMutex    sync.Mutex
)

func init() {
	subscribers = make(map[net.Conn]bool)
}

// AddSubscriber registers a new GUI/CLI listener
func AddSubscriber(conn net.Conn) {
	subMutex.Lock()
	defer subMutex.Unlock()
	subscribers[conn] = true
	fmt.Println("📡 New Event Subscriber connected.")
}

// RemoveSubscriber removes a GUI/CLI listener
func RemoveSubscriber(conn net.Conn) {
	subMutex.Lock()
	defer subMutex.Unlock()
	delete(subscribers, conn)
	fmt.Println("📡 Event Subscriber disconnected.")
}

func BroadcastEvent(eventMessage string) {
	subMutex.Lock()
	defer subMutex.Unlock()

	for conn := range subscribers {
		_, err := conn.Write([]byte(eventMessage + "\n"))
		if err != nil {
			// If the GUI closed or crashed, remove them from the list
			conn.Close()
			delete(subscribers, conn)
		}
	}
}

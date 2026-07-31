package api // Adjust based on your structure

import (
	"claviger-server/storage"
	"database/sql"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// HandleFleetProxy forwards any API request from the Master UI to the target Sub-Server
func HandleFleetProxy(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 1. Identify the Target Node via HTTP Headers
		// The UI will pass the target node's ID in this custom header
		targetNodeID := r.Header.Get("X-Node-ID")
		if targetNodeID == "" {
			http.Error(w, "Missing X-Node-ID header", http.StatusBadRequest)
			return
		}

		// 2. Determine what endpoint we are trying to hit on the Sub-server
		// Example: If Master URL is /api/fleet/proxy/api/system
		// The target path becomes /api/system
		targetPath := strings.TrimPrefix(r.URL.Path, "/api/fleet/proxy")
		if targetPath == "" {
			http.Error(w, "Missing target path", http.StatusBadRequest)
			return
		}

		// Preserve query parameters (e.g., ?page=1)
		if r.URL.RawQuery != "" {
			targetPath += "?" + r.URL.RawQuery
		}

		// 3. Look up the Node's VPN IP and API Key
		var nodeIP, apiKey string
		query := `
			SELECT c.ip_address, s.api_key 
			FROM sub_servers s
			JOIN clients c ON s.client_id = c.id
			WHERE s.id = ? AND s.status = 'active'`

		err := db.QueryRow(query, targetNodeID).Scan(&nodeIP, &apiKey)
		if err != nil {
			if err == sql.ErrNoRows {
				http.Error(w, "Node is offline, pending, or does not exist", http.StatusNotFound)
			} else {
				log.Printf("❌ Database error looking up node %s: %v", targetNodeID, err)
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
			return
		}

		// 4. Get the Sub-server's listening port (default to 18080 if not defined globally)
		targetPort := storage.GetConfig(db, "hub_port")
		if targetPort == "" {
			targetPort = "18080"
		}

		// 5. Construct the full URL to the Sub-server over the VPN
		targetURL := fmt.Sprintf("http://%s:%s%s", nodeIP, targetPort, targetPath)

		// 6. Create a new HTTP request mirroring the exact method and body from the UI
		proxyReq, err := http.NewRequest(r.Method, targetURL, r.Body)
		if err != nil {
			http.Error(w, "Failed to create proxy request", http.StatusInternalServerError)
			return
		}

		// 7. Copy Original Headers (like Content-Type) & Attach Zero Trust Credentials
		for key, values := range r.Header {
			for _, value := range values {
				// Don't copy the custom node ID header to the sub-server
				if key != "X-Node-ID" {
					proxyReq.Header.Add(key, value)
				}
			}
		}
		proxyReq.Header.Set("Authorization", "Bearer "+apiKey)

		// 8. Execute the Request with a Strict Timeout
		// We use 10 seconds. If the node is powered off, we don't want the Master UI to hang forever.
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Do(proxyReq)
		if err != nil {
			log.Printf("⚠️ Failed to reach Sub-server at %s: %v", targetURL, err)
			http.Error(w, "Sub-server unreachable (Node might be offline)", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		// 9. Pipe the Sub-server's exact response back to the Master UI
		// Copy all response headers
		for key, values := range resp.Header {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
		// Set the exact status code returned by the sub-server (e.g., 200 OK, 400 Bad Request)
		w.WriteHeader(resp.StatusCode)

		// Stream the body data
		io.Copy(w, resp.Body)
	}
}

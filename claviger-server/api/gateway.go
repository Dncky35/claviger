package api

import (
	"encoding/json"
	"net/http"

	"claviger-server/internal/gateway"
)

// GatewayStatusResp tells the UI what the server's current network state is
type GatewayStatusResp struct {
	IsRunning  bool   `json:"is_running"`
	PortsClear bool   `json:"ports_clear"`
	Message    string `json:"message"`
}

// HandleGatewayStatus is a GET request called when the user opens the Network tab
func HandleGatewayStatus() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		if r.Method != http.MethodGet {
			http.Error(w, `{"error": "Method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}

		isRunning := gateway.IsGatewayRunning()

		// If it's already running, we don't need to check ports
		if isRunning {
			json.NewEncoder(w).Encode(GatewayStatusResp{
				IsRunning:  true,
				PortsClear: false, // Irrelevant if already running
				Message:    "Master Gateway is active and managing traffic.",
			})
			return
		}

		// If not running, check if it's safe to install
		err := gateway.CheckPorts()
		portsClear := err == nil
		msg := "Ready to install Master Gateway."
		if !portsClear {
			msg = err.Error() // Sends the exact conflict message (e.g., "Port 80 in use")
		}

		json.NewEncoder(w).Encode(GatewayStatusResp{
			IsRunning:  false,
			PortsClear: portsClear,
			Message:    msg,
		})
	}
}

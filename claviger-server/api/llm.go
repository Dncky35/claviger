package api

import (
	"claviger-server/internal/hardware"
	"encoding/json"
	"net/http"
)

// /api/hardware/ai
func HandleAIHardware(w http.ResponseWriter, r *http.Request) {

	if r.Method == http.MethodGet {
		// 1. Call the native Go profiler we already wrote
		profile, err := hardware.RunProfiler()

		if err != nil {
			http.Error(w, "Failed to profile hardware", http.StatusInternalServerError)
			return
		}

		// 2. Return the profile as JSON
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(profile)
	}

}

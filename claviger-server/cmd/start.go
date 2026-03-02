package cmd

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"claviger-server/storage"
	"claviger-server/web"
)

func RunStart() {
	fmt.Println("=== Starting Claviger Edge Daemon ===")

	// 1. Initialize DB to read local config
	db := storage.InitDB()
	defer db.Close()

	nodeID := storage.GetConfig(db, "node_id")
	apiToken := storage.GetConfig(db, "api_token")

	if apiToken == "" {
		log.Println("⚠️  Warning: Node is not registered. Run 'claviger setup' first.")
	} else {
		log.Println("✅ Node Identity loaded.")
	}

	// 2. Setup the Local API Endpoint
	http.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		status := map[string]interface{}{
			"node_id":   nodeID,
			"has_token": apiToken != "",
			"status":    "running",
		}

		json.NewEncoder(w).Encode(status)
	})

	// 3. Serve the Embedded Hub UI
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		// Use the variable from your web package!
		w.Write(web.IndexHTML)
	})

	port := "18080"
	fmt.Printf("🌐 Local Hub running at: http://127.0.0.1:%s\n", port)

	err := http.ListenAndServe("127.0.0.1:"+port, nil)
	if err != nil {
		log.Fatalf("❌ Failed to start local server: %v", err)
	}
}

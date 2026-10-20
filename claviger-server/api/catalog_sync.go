package api

import (
	"claviger-server/internal/apps"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
)

// HandleManualSync handles POST /api/apps/sync to manually trigger catalog synchronization
func HandleManualSync(db *sql.DB) http.HandlerFunc {
	// Initialize the syncer once when the route is registered
	syncer := apps.NewCatalogSyncer(db)

	return func(w http.ResponseWriter, r *http.Request) {
		// 1. Enforce POST method (Security best practice for state-changing actions)
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed. Use POST.", http.StatusMethodNotAllowed)
			return
		}

		log.Println("🔄 User initiated manual catalog sync...")

		// 2. Trigger the sync logic
		status, err := syncer.Sync()
		w.Header().Set("Content-Type", "application/json")

		// 3. Handle Errors
		if err != nil {
			log.Printf("⚠️ Manual catalog sync failed: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{
				"status":  "error",
				"message": err.Error(),
			})
			return
		}

		// 4. Map the status to user-friendly UI messages
		messages := map[string]string{
			"updated":    "Latest apps synced securely from CloudROcean!",
			"up_to_date": "Catalog is already up to date.",
		}

		msg, exists := messages[status]
		if !exists {
			msg = "Catalog sync completed." // Safe fallback
		}

		log.Printf("✅ Manual catalog sync completed: %s", status)

		// 5. Return Success
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"status":  status,
			"message": msg,
		})
	}
}

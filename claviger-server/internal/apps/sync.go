package apps

import (
	"claviger-server/internal/types"
	"claviger-server/storage"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

const (
	SyncURL       = "https://api.cloudrocean.com/v1/claviger-apps/manifest"
	ETagConfigKey = "remote_catalog_etag"
)

type CatalogSyncer struct {
	db         *sql.DB
	httpClient *http.Client
}

func NewCatalogSyncer(db *sql.DB) *CatalogSyncer {
	return &CatalogSyncer{
		db: db,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// Sync performs an HTTP check with ETag caching
func (s *CatalogSyncer) Sync() (string, error) {
	req, err := http.NewRequest("GET", SyncURL, nil)
	if err != nil {
		return "error", fmt.Errorf("failed to create request: %w", err)
	}

	// Read stored ETag from SQLite config table
	currentETag := storage.GetConfig(s.db, ETagConfigKey)
	if currentETag != "" {
		req.Header.Set("If-None-Match", currentETag)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "error", fmt.Errorf("network error during catalog sync: %w", err)
	}
	defer resp.Body.Close()

	// 304 Not Modified: Cache is up to date!
	if resp.StatusCode == http.StatusNotModified {
		log.Println("⚡ [Catalog Sync] Remote catalog unchanged (304 Not Modified).")
		return "up_to_date", nil
	}

	if resp.StatusCode != http.StatusOK {
		return "error", fmt.Errorf("unexpected status code from cloudrocean.com: %d", resp.StatusCode)
	}

	// 200 OK: New data available
	var groupedCatalog map[string][]types.AppManifest
	if err := json.NewDecoder(resp.Body).Decode(&groupedCatalog); err != nil {
		return "error", fmt.Errorf("failed to decode catalog JSON: %w", err)
	}

	// Save to remote_apps table
	if err := storage.UpsertRemoteApps(s.db, groupedCatalog); err != nil {
		return "error", fmt.Errorf("failed to update local database: %w", err)
	}

	// Save new ETag
	newETag := resp.Header.Get("ETag")
	if newETag != "" {
		_ = storage.SetConfig(s.db, ETagConfigKey, newETag)
	}

	log.Println("✅ [Catalog Sync] Remote catalog updated successfully.")
	return "updated", nil
}

// StartBackgroundPoller runs every 12 hours in a goroutine
func (s *CatalogSyncer) StartBackgroundPoller(interval time.Duration) {
	go func() {
		// Run initial check on startup
		if _, err := s.Sync(); err != nil {
			log.Printf("⚠️ [Catalog Sync] Initial background sync failed: %v", err)
		}

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for range ticker.C {
			if _, err := s.Sync(); err != nil {
				log.Printf("⚠️ [Catalog Sync] Scheduled sync failed: %v", err)
			}
		}
	}()
}

// HandleManualSync HTTP Handler for POST /api/apps/sync
func HandleManualSync(db *sql.DB) http.HandlerFunc {
	syncer := NewCatalogSyncer(db)
	return func(w http.ResponseWriter, r *http.Request) {
		status, err := syncer.Sync()

		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{
				"status":  "error",
				"message": err.Error(),
			})
			return
		}

		json.NewEncoder(w).Encode(map[string]string{
			"status": status,
			"message": map[string]string{
				"updated":    "Latest apps synced from cloudrocean.com!",
				"up_to_date": "Catalog is already up to date.",
			}[status],
		})
	}
}

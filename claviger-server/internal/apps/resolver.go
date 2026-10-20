package apps

import (
	"claviger-server/internal/types"
	"claviger-server/storage"
	"database/sql"
	"fmt"
)

// GetManifest resolves an app manifest following the 3-tier precedence hierarchy:
// Tier 1: Custom local apps (SQLite)
// Tier 2: Synced remote apps (SQLite)
// Tier 3: Compiled static catalog (Go Memory)
func GetManifest(db *sql.DB, appID string) (types.AppManifest, error) {
	//1. Check custom_apps (Tier 1)

	customApp, foundCustom, err := storage.GetCustomAppByID(db, appID)
	if err == nil && foundCustom {
		return customApp, nil
	}

	// 2. Check remote_apps (Tier 2)
	remoteApp, foundRemote, err := storage.GetRemoteAppByID(db, appID)
	if err == nil && foundRemote {
		return remoteApp, nil
	}

	// 3. Check compiled Catalog (Tier 3)
	if staticApp, foundStatic := FallBackCatalog[appID]; foundStatic {
		return staticApp, nil
	}

	return types.AppManifest{}, fmt.Errorf("app [%s] not found in local, remote, or system catalog", appID)
}

// GetCombinedCatalog returns the merged list of all available apps for the Web UI
func GetCombinedCatalog(db *sql.DB) (map[string]types.AppManifest, error) {
	combined := make(map[string]types.AppManifest)

	// Base Layer: Hardcoded compiled catalog
	for id, manifest := range FallBackCatalog {
		combined[id] = manifest
	}

	// Override / Enrich Layer: Synced remote apps from cloudrocean.com
	remoteApps, err := storage.GetRemoteApps(db)
	if err == nil {
		for id, manifest := range remoteApps {
			combined[id] = manifest
		}
	}

	// Top Layer: User's custom apps
	customApps, err := storage.GetCustomApps(db)
	if err == nil {
		for id, manifest := range customApps {
			combined[id] = manifest
		}
	}

	return combined, nil
}

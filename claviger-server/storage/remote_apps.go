package storage

import (
	"claviger-server/internal/types"
	"database/sql"
	"fmt"
	"time"
)

func UpsertRemoteApps(db *sql.DB, catalog map[string][]types.AppManifest) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to start upsert transaction: %w", err)
	}

	// Option A: Clear old remote cache and replace with fresh snapshot
	_, err = tx.Exec("DELETE FROM remote_apps;")
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to clear remote_apps cache: %w", err)
	}

	stmt, err := tx.Prepare(`
		INSERT INTO remote_apps (
			id, catalog_type, name, category, description, icon, 
			needs_dynamic_port, has_custom_setup, setup_port, compose_yaml, 
			version_tag, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
	`)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to prepare insert statement: %w", err)
	}
	defer stmt.Close()

	now := time.Now().UTC()

	// Iterate over each catalog_type slice ("default", "llm", "homelab", etc.)
	for catalogType, appList := range catalog {
		for _, app := range appList {
			_, err = stmt.Exec(
				app.ID,
				catalogType,
				app.Name,
				app.Category,
				app.Description,
				app.Icon,
				app.NeedsDynamicPort,
				app.HasCustomSetup,
				app.SetupPort,
				app.ComposeYAML,
				app.VersionTag,
				now,
			)
			if err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to insert remote app [%s]: %w", app.ID, err)
			}
		}
	}

	return tx.Commit()
}

// GetRemoteApps fetches all cached remote apps from SQLite
func GetRemoteApps(db *sql.DB) (map[string]types.AppManifest, error) {
	rows, err := db.Query(`
		SELECT id, catalog_type, name, category, description, icon, 
		       needs_dynamic_port, has_custom_setup, setup_port, compose_yaml, version_tag 
		FROM remote_apps;
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query remote_apps: %w", err)
	}
	defer rows.Close()

	remoteApps := make(map[string]types.AppManifest)

	for rows.Next() {
		var manifest types.AppManifest
		err := rows.Scan(
			&manifest.ID,
			&manifest.CatalogType,
			&manifest.Name,
			&manifest.Category,
			&manifest.Description,
			&manifest.Icon,
			&manifest.NeedsDynamicPort,
			&manifest.HasCustomSetup,
			&manifest.SetupPort,
			&manifest.ComposeYAML,
			&manifest.VersionTag,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan remote app row: %w", err)
		}
		remoteApps[manifest.ID] = manifest
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate remote_apps rows: %w", err)
	}

	return remoteApps, nil
}

// GetRemoteAppByID fetches a single remote app from SQLite
func GetRemoteAppByID(db *sql.DB, appID string) (types.AppManifest, bool, error) {
	row := db.QueryRow(`
		SELECT id, catalog_type, name, category, description, icon, 
		       needs_dynamic_port, has_custom_setup, setup_port, compose_yaml, version_tag 
		FROM remote_apps 
		WHERE id = ?;
	`, appID)

	var manifest types.AppManifest
	err := row.Scan(
		&manifest.ID,
		&manifest.CatalogType,
		&manifest.Name,
		&manifest.Category,
		&manifest.Description,
		&manifest.Icon,
		&manifest.NeedsDynamicPort,
		&manifest.HasCustomSetup,
		&manifest.SetupPort,
		&manifest.ComposeYAML,
		&manifest.VersionTag,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return manifest, false, nil
		}
		return manifest, false, fmt.Errorf("failed to query remote app [%s]: %w", appID, err)
	}

	return manifest, true, nil
}

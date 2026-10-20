package storage

import (
	"claviger-server/internal/types"
	"database/sql"
	"fmt"
)

func GetCustomAppByID(db *sql.DB, appID string) (types.AppManifest, bool, error) {
	row := db.QueryRow(`
		SELECT id, name, description, icon, 
		       needs_dynamic_port, has_custom_setup, setup_port, compose_yaml 
		FROM custom_apps 
		WHERE id = ?
	`, appID)

	var manifest types.AppManifest
	var desc, icon sql.NullString // Safely handle potential NULLs from older data

	err := row.Scan(
		&manifest.ID,
		&manifest.Name,
		&desc,
		&icon,
		&manifest.NeedsDynamicPort,
		&manifest.HasCustomSetup,
		&manifest.SetupPort,
		&manifest.ComposeYAML,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return manifest, false, nil
		}
		return manifest, false, fmt.Errorf("failed to query custom app [%s]: %w", appID, err)
	}

	manifest.Description = desc.String
	manifest.Icon = icon.String

	// Map the legacy table to the unified struct
	manifest.CatalogType = "custom"
	manifest.Category = "custom"
	manifest.VersionTag = "local"

	return manifest, true, nil
}

func GetCustomApps(db *sql.DB) (map[string]types.AppManifest, error) {
	rows, err := db.Query(`
		SELECT id, name, description, icon, 
		       needs_dynamic_port, has_custom_setup, setup_port, compose_yaml 
		FROM custom_apps;
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query custom_apps: %w", err)
	}
	defer rows.Close()

	customApps := make(map[string]types.AppManifest)

	for rows.Next() {
		var manifest types.AppManifest
		var desc, icon sql.NullString

		err := rows.Scan(
			&manifest.ID,
			&manifest.Name,
			&desc,
			&icon,
			&manifest.NeedsDynamicPort,
			&manifest.HasCustomSetup,
			&manifest.SetupPort,
			&manifest.ComposeYAML,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan custom app row: %w", err)
		}

		manifest.Description = desc.String
		manifest.Icon = icon.String
		manifest.CatalogType = "custom"
		manifest.Category = "custom"
		manifest.VersionTag = "local"

		customApps[manifest.ID] = manifest
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate custom_apps rows: %w", err)
	}

	return customApps, nil
}

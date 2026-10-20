package types

import "time"

// AppManifest defines the universal schema used by Claviger's runner
type AppManifest struct {
	ID               string `json:"app_id"`
	CatalogType      string `json:"catalog_type,omitempty"`
	Name             string `json:"name"`
	Category         string `json:"category"`
	Description      string `json:"description,omitempty"`
	Icon             string `json:"icon,omitempty"`
	HasCustomSetup   bool   `json:"has_custom_setup"`
	NeedsDynamicPort bool   `json:"needs_dynamic_port"`
	SetupPort        int    `json:"setup_port"`
	ComposeYAML      string `json:"compose_yaml"`
	VersionTag       string `json:"version_tag,omitempty"`
}

// RemoteApp represents an entry synced from cloudrocean.com in SQLite
type RemoteApp struct {
	ID               string    `db:"app_id"`
	CatalogType      string    `db:"catalog_type"`
	Name             string    `db:"name"`
	Category         string    `db:"category"`
	Description      string    `db:"description"`
	Icon             string    `db:"icon"`
	NeedsDynamicPort bool      `db:"needs_dynamic_port"`
	HasCustomSetup   bool      `db:"has_custom_setup"`
	SetupPort        int       `db:"setup_port"`
	ComposeYAML      string    `db:"compose_yaml"`
	VersionTag       string    `db:"version_tag"`
	UpdatedAt        time.Time `db:"updated_at"`
}

// ToManifest converts a RemoteApp into a standard AppManifest
func (r *RemoteApp) ToManifest() AppManifest {
	return AppManifest{
		ID:               r.ID,
		CatalogType:      r.CatalogType,
		Name:             r.Name,
		Category:         r.Category,
		Description:      r.Description,
		Icon:             r.Icon,
		NeedsDynamicPort: r.NeedsDynamicPort,
		HasCustomSetup:   r.HasCustomSetup,
		SetupPort:        r.SetupPort,
		ComposeYAML:      r.ComposeYAML,
		VersionTag:       r.VersionTag,
	}
}

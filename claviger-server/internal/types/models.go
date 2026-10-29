package types

import "time"

// AppManifest defines the universal schema used by Claviger's runner
type AppManifest struct {
	ID               string  `json:"app_id"`
	CatalogType      string  `json:"catalog_type,omitempty"`
	Name             string  `json:"name"`
	Category         string  `json:"category"`
	Description      string  `json:"description,omitempty"`
	Icon             string  `json:"icon,omitempty"`
	HasCustomSetup   bool    `json:"has_custom_setup"`
	NeedsDynamicPort bool    `json:"needs_dynamic_port"`
	SetupPort        int     `json:"setup_port"`
	ComposeYAML      string  `json:"compose_yaml"`
	ConfigFile       *string `json:"config_file"`
	VersionTag       string  `json:"version_tag,omitempty"`
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
	ConfigFile       *string   `db:"config_file" json:"config_file"` // FIXED: Added db tag for consistency
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
		ConfigFile:       r.ConfigFile, // FIXED: Now properly passes the schema payload
		VersionTag:       r.VersionTag,
	}
}

// Define the core Action struct if you haven't already
type Action struct {
	Type            string            `json:"type"` // "env", "exec", "api"
	TargetKey       string            `json:"target_key,omitempty"`
	ValueMapping    map[string]string `json:"value_mapping,omitempty"`
	RequiresRestart bool              `json:"requires_restart"`
	ContainerName   string            `json:"container_name,omitempty"`
	RunAsUser       string            `json:"run_as_user,omitempty"`
	CommandTemplate string            `json:"command_template,omitempty"`
}

// UpdatePayload represents what the frontend POSTs to the backend.
type UpdatePayload struct {
	AppID    string `json:"app_id"`
	FieldID  string `json:"field_id"`
	NewValue string `json:"new_value"`
}

// Structs to unmarshal the schema.json
type AppSchema struct {
	AppID      string           `json:"app_id"`
	Categories []SchemaCategory `json:"categories"`
}

type SchemaCategory struct {
	ID     string        `json:"id"`
	Title  string        `json:"title"`
	Fields []SchemaField `json:"fields"`
}

type SchemaField struct {
	ID           string       `json:"id"`
	Label        string       `json:"label"`
	Description  string       `json:"description"`
	UIComponent  string       `json:"ui_component"`  // "toggle", "select", "text", "password"
	DataType     string       `json:"data_type"`     // "boolean", "string"
	CurrentValue interface{}  `json:"current_value"` // interface{} because it can be bool or string
	Options      []OptionItem `json:"options,omitempty"`
	Action       Action       `json:"action"`
}

type OptionItem struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

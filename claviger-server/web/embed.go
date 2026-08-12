package web

import (
	"embed"
)

//go:embed index.html components/*.html static/* locales/*.json
var TemplatesFS embed.FS

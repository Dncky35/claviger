package web

import (
	"embed"
)

//go:embed index.html components/*.html
var TemplatesFS embed.FS

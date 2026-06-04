package web

import (
	"embed"
)

//go:embed index.html components/*.html static/*
var TemplatesFS embed.FS

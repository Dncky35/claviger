package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// Create an instance of the app structure
	app := NewApp()

	// Create application with options
	err := wails.Run(&options.App{
		Title:  "Claviger Zero Trust",
		Width:  400, // Make it look like a sleek VPN app (tall and narrow)
		Height: 600,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 15, G: 23, B: 42, A: 1}, // Tailwind slate-900
		OnStartup:        app.startup,
		Bind: []interface{}{
			app, // This binds all our app.go functions to JavaScript!
		},
	})

	if err != nil {
		log.Fatal("Error starting Claviger Client:", err)
	}
}

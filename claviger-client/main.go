package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/src
var assets embed.FS

func main() {
	// Create an instance of the app structure
	app := NewApp()

	// Create application with options
	err := wails.Run(&options.App{
		Title:     "Claviger Zero Trust",
		Width:     900, // 👈 Wide desktop layout
		Height:    550, // 👈 Slightly shorter so it looks like a sleek dashboard
		MinWidth:  800, // 👈 Prevents the user from squishing the UI
		MinHeight: 500,
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

package main

import (
	"embed"
	"fmt"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"unified-inbox/internal/inbox"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	dir, err := dataDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "Failed to create data directory:", err)
		os.Exit(1)
	}
	log := newLogger(dir)
	v := versionInfo()
	log.Info().Str("version", v.Version).Str("commit", v.Commit).Str("branch", v.Branch).
		Str("build_date", v.BuildDate).Msg("Starting Pigeonhole")
	svc, err := inbox.New(dir, log)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize inbox")
	}
	settings := loadSettings(dir)
	app := NewApp(svc, dir, settings)
	background, macOptions, windowsOptions := windowAppearance(settings.Theme)

	err = wails.Run(&options.App{
		Title:     "Pigeonhole",
		Width:     1180,
		Height:    780,
		MinWidth:  720,
		MinHeight: 480,
		AssetServer: &assetserver.Options{
			Assets:  assets,
			Handler: svc.MediaHandler(),
		},
		BackgroundColour: background,
		Mac:              macOptions,
		Windows:          windowsOptions,
		DragAndDrop: &options.DragAndDrop{
			EnableFileDrop:     true,
			DisableWebViewDrop: true,
		},
		OnStartup:  app.startup,
		OnShutdown: app.shutdown,
		Bind: []interface{}{
			app,
		},
	})
	if err != nil {
		log.Err(err).Msg("Wails exited with error")
	}
}

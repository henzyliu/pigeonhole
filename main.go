package main

import (
	"embed"
	"fmt"
	"os"
	goruntime "runtime"
	"slices"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"unified-inbox/internal/autostart"
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
	// Launched at login (see internal/autostart): start hidden.
	background := slices.Contains(os.Args[1:], autostart.BackgroundFlag)
	app := NewApp(svc, dir, settings, background)
	backgroundColour, macOptions, windowsOptions := windowAppearance(settings.Theme)

	enableSpellChecking()
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
		BackgroundColour: backgroundColour,
		// Closing the window keeps Pigeonhole running; quit from the menu
		// bar / tray icon (or ⌘Q on macOS). See background.go.
		HideWindowOnClose: true,
		// macOS hides the app itself when started at login (so the Dock
		// icon still brings it back); Windows starts the window hidden.
		StartHidden: background && goruntime.GOOS == "windows",
		// Opening Pigeonhole again shows the running copy.
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               "com.pigeonhole.app",
			OnSecondInstanceLaunch: func(options.SecondInstanceData) { app.showWindow() },
		},
		Mac:     macOptions,
		Windows: windowsOptions,
		// The right-click menu shows only where it's useful: in text boxes
		// (with spelling suggestions) and on selected text, so you can copy
		// it. See --default-contextmenu in the Wails runtime.
		EnableDefaultContextMenu: true,
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

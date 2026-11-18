//go:build windows && gui

// Command stemscribe-gui is the desktop client: double-click, get a window.
//
// It is a separate binary from cmd/stemscribe on purpose. The native shell needs
// cgo + the WebView2 loader, and putting that in the same package would force
// every `go build ./...` (and the CI job) to carry a C toolchain. The build tag
// keeps the CLI pure Go while the shipped app is a real window.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/hhx465453939/stemscribe/internal/webui"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

// version is stamped at build time via -ldflags "-X main.version=..."
var version = "0.0.1-dev"

// App is the binding surface exposed to the UI as window.go.main.App.
type App struct {
	ctx context.Context
}

func (a *App) startup(ctx context.Context) { a.ctx = ctx }

// Version is read by the settings page.
func (a *App) Version() string { return version }

func main() {
	ui, err := webui.FS()
	if err != nil || !webui.Embedded() {
		// No silent blank window: say what to do and exit.
		fmt.Fprintln(os.Stderr, "UI assets are missing from this build.")
		fmt.Fprintln(os.Stderr, "run: pwsh scripts/build.ps1")
		os.Exit(1)
	}

	app := &App{}
	err = wails.Run(&options.App{
		Title:  "Stemscribe",
		Width:  1280,
		Height: 860,
		// Matches --bg in globals.css so the frame does not flash white before the
		// webview paints. A light window around a dark page looks broken.
		BackgroundColour: &options.RGBA{R: 10, G: 11, B: 13, A: 255},
		AssetServer:      &assetserver.Options{Assets: ui},
		OnStartup:        app.startup,
		Bind:             []any{app},
		Windows: &windows.Options{
			// Keep the native frame: the UI is a tool window, not a splash screen.
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			DisableWindowIcon:    false,
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "stemscribe: %v\n", err)
		os.Exit(1)
	}
}

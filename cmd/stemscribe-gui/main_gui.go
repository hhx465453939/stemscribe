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
	"net/http"
	"os"
	"os/exec"

	"github.com/hhx465453939/stemscribe/internal/config"
	"github.com/hhx465453939/stemscribe/internal/runctl"
	"github.com/hhx465453939/stemscribe/internal/webapi"
	"github.com/hhx465453939/stemscribe/internal/webui"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"
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

// PickFiles opens the native multi-file picker.
//
// This exists because a browser cannot hand us an absolute path: the file input gives
// bytes, and copying a multi-gigabyte video through the page just to learn its name
// would be the wrong solution. Only the desktop window can do this properly.
func (a *App) PickFiles() ([]string, error) {
	return runtime.OpenMultipleFilesDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select media files",
		Filters: []runtime.FileFilter{
			{DisplayName: "Media", Pattern: "*.mp4;*.mkv;*.mov;*.webm;*.avi;*.wav;*.mp3;*.m4a;*.flac"},
		},
	})
}

// OpenFolder reveals a bundle in the system file manager.
func (a *App) OpenFolder(path string) error {
	if path == "" {
		return fmt.Errorf("empty path")
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("folder is gone: %w", err)
	}
	return exec.Command("explorer", path).Start()
}

func main() {
	ui, err := webui.FS()
	if err != nil || !webui.Embedded() {
		// No silent blank window: say what to do and exit.
		fmt.Fprintln(os.Stderr, "UI assets are missing from this build.")
		fmt.Fprintln(os.Stderr, "run: pwsh scripts/build.ps1")
		os.Exit(1)
	}

	app := &App{}

	// The window serves the exact same handler as the loopback shell, so "the button
	// works in the browser" and "the button works in the window" are one claim, not
	// two. Before this, the window had no run path at all: clicking Start did nothing.
	runctl.Version = version
	svc := runctl.NewService()
	wd, _ := config.DefaultWorkdir()
	handler := webapi.New(svc, http.FileServer(http.FS(ui)), wd)

	err = wails.Run(&options.App{
		Title:  "Stemscribe",
		Width:  1280,
		Height: 860,
		// Matches --bg in globals.css so the frame does not flash white before the
		// webview paints. A light window around a dark page looks broken.
		BackgroundColour: &options.RGBA{R: 10, G: 11, B: 13, A: 255},
		AssetServer:      &assetserver.Options{Assets: ui, Handler: handler},
		OnStartup:        app.startup,
		Bind:             []any{app},
		// Dropping files onto the window is a command, not a text fill-in (ADR-040):
		// the page registers runtime.OnFileDrop and receives absolute paths, then
		// enqueues them immediately. Wails only forwards OS drops when this is on.
		DragAndDrop: &options.DragAndDrop{EnableFileDrop: true},
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

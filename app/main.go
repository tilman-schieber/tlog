// Command tlog-app is the desktop outliner: a third adapter over the same core
// the TUI and the CLI use. It holds no logic of its own — every operation it
// offers is a method on app.Service.
//
// It renders in the system WebView (WKWebView on macOS, WebKitGTK on Linux),
// so the binary is a few megabytes and nothing is bundled.
package main

import (
	"context"
	"embed"
	"io/fs"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"

	tapp "github.com/tilman-schieber/tlog/internal/app"
)

//go:embed all:frontend
var assets embed.FS

func main() {
	svc, err := tapp.New("")
	if err != nil {
		log.Fatal(err)
	}

	ui, err := fs.Sub(assets, "frontend")
	if err != nil {
		log.Fatal(err)
	}

	api := &API{svc: svc}

	err = wails.Run(&options.App{
		Title:         "tlog",
		Width:         1100,
		Height:        760,
		MinWidth:      520,
		MinHeight:     360,
		AssetServer:   &assetserver.Options{Assets: ui},
		Bind:          []any{api},
		OnBeforeClose: api.onClose,
		OnStartup:     func(ctx context.Context) { api.ctx = ctx },
		Mac: &mac.Options{
			TitleBar:             mac.TitleBarHiddenInset(),
			WebviewIsTransparent: false,
			About: &mac.AboutInfo{
				Title:   "tlog",
				Message: "A markdown-first knowledge outliner.\nYour notes are plain files.",
			},
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}

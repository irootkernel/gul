package desktop

import (
	"github.com/rootkernel/gul/internal/delivery/web"
	"github.com/wailsapp/wails/v3/pkg/application"
)

type WailsHost struct{}

func (WailsHost) Run(onShutdown func()) error {
	app := application.New(wailsOptions(onShutdown))
	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:   "main",
		Title:  "Gul",
		Width:  1100,
		Height: 760,
		URL:    "/",
	})
	return app.Run()
}

func wailsOptions(onShutdown func()) application.Options {
	return application.Options{
		Name:       "Gul",
		OnShutdown: onShutdown,
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
		Assets: application.AssetOptions{Handler: web.BrowserHandler()},
	}
}

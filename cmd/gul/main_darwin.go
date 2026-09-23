package main

import (
	"context"
	"log"

	"github.com/rootkernel/gul/internal/app"
	"github.com/rootkernel/gul/internal/desktop"
)

func main() {
	core := app.NewCore(app.Dependencies{})
	if err := desktop.NewShell(core, desktop.WailsHost{}).Run(context.Background()); err != nil {
		log.Fatal(err)
	}
}

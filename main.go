package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/digimata/gh-render/internal/app"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	os.Exit(app.Run(
		ctx,
		os.Args[1:],
		os.Stdout,
		os.Stderr,
		app.DefaultDependencies(),
	))
}

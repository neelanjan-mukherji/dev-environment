// Command dev sets up and maintains a machine from the dev-environment repo.
package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/neelanjan-mukherji/dev-environment/cli/internal/app"
	"github.com/neelanjan-mukherji/dev-environment/cli/internal/sys"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := app.Main(ctx, os.Args[1:], os.Stdout, os.Stderr, sys.Exec{})
	stop()
	os.Exit(code)
}

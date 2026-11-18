// Command dev sets up and maintains a machine from the dev-environment repo.
package main

import (
	"context"
	"os"
	"os/signal"
	"runtime"

	"github.com/neelanjan-mukherji/dev-environment/cli/internal/app"
	"github.com/neelanjan-mukherji/dev-environment/cli/internal/sys"
	"github.com/neelanjan-mukherji/dev-environment/cli/internal/ui"
)

// version is set by release builds (.goreleaser.yaml).
var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := app.Main(ctx, os.Args[1:], os.Stdout, os.Stderr, app.Deps{Runner: sys.Exec{}, Prompter: ui.Terminal{}, Version: version, OS: runtime.GOOS})
	stop()
	os.Exit(code)
}

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
	if runtime.GOOS == "windows" {
		// Started from PowerShell 7, dev inherits its PSModulePath; Windows
		// PowerShell children (chezmoi's scripts, doctor's checks) then load 7's
		// type data and fail ("The member ... is already present"). Unset, each
		// PowerShell builds its own default.
		os.Unsetenv("PSModulePath")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := app.Main(ctx, os.Args[1:], os.Stdout, os.Stderr, app.Deps{Runner: sys.Exec{}, Prompter: ui.Terminal{}, Version: version, OS: runtime.GOOS})
	stop()
	os.Exit(code)
}

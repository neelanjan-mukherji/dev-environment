//go:build unix

package sys_test

import (
	"context"
	"os"
	"testing"

	"github.com/neelanjan-mukherji/dev-environment/cli/internal/sys"
)

// `dev doctor` runs `zsh -ic` checks; with a terminal they took it over for job
// control and the shell stopped dev ("suspended (tty input)").
func TestCapturedCommandsCannotReachTheTerminal(t *testing.T) {
	tty, err := os.Open("/dev/tty")
	if err != nil {
		t.Skip("no terminal to protect (run under `script` to exercise this)")
	}
	tty.Close()
	if out, err := (sys.Exec{}).Run(context.Background(), sys.Command("sh", "-c", ": </dev/tty")); err == nil {
		t.Fatalf("a captured command opened the terminal\n%s", out)
	}
}

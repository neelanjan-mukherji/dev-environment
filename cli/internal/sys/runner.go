// Package sys is the boundary between dev and the machine: every external
// command dev runs goes through a Runner, so commands can be faked in tests.
package sys

import (
	"context"
	"os"
	"os/exec"
)

// Runner runs a command and returns its combined output. A non-zero exit is an error.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) (string, error)
}

// Exec runs real commands from the home directory (so a project's mise.toml
// in the current directory doesn't change what the checks see), with no stdin.
type Exec struct{}

func (Exec) Run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if home, err := os.UserHomeDir(); err == nil {
		cmd.Dir = home
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

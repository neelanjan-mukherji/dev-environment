// Package sys is the boundary between dev and the machine: every external
// command dev runs goes through a Runner, so commands can be faked in tests.
package sys

import (
	"context"
	"os"
	"os/exec"
)

// Cmd is one external command.
type Cmd struct {
	Name string
	Args []string
	// Env is added to dev's own environment ("KEY=value").
	Env []string
	// Attach connects the command to the terminal (it may prompt; its output is
	// shown live and not captured). Otherwise it runs with no stdin and its
	// combined output is returned.
	Attach bool
}

// Command is a captured command without extra environment.
func Command(name string, args ...string) Cmd { return Cmd{Name: name, Args: args} }

// Runner runs commands. A non-zero exit is an error.
type Runner interface {
	Run(ctx context.Context, c Cmd) (string, error)
}

// Exec runs real commands. Captured commands run from the home directory, so a
// project's mise.toml in the current directory doesn't change what they see.
type Exec struct{}

func (Exec) Run(ctx context.Context, c Cmd) (string, error) {
	cmd := exec.CommandContext(ctx, c.Name, c.Args...)
	if len(c.Env) > 0 {
		cmd.Env = append(os.Environ(), c.Env...)
	}
	if c.Attach {
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		return "", cmd.Run()
	}
	if home, err := os.UserHomeDir(); err == nil {
		cmd.Dir = home
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

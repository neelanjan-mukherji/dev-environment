//go:build unix

package sys

import (
	"os/exec"
	"syscall"
)

// detach runs a captured command in its own session, without a controlling
// terminal: an interactive shell (`zsh -ic`) would otherwise take the terminal
// over for job control and leave dev stopped in the background.
func detach(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }

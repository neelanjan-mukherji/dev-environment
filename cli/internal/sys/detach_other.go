//go:build !unix

package sys

import "os/exec"

func detach(*exec.Cmd) {}

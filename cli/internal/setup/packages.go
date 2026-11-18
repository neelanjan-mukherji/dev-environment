package setup

import (
	"context"
	"errors"
	"os"
	"strings"
)

// packageManager is how one Linux family installs and upgrades packages. The
// base package lists are the chezmoi scripts' business (packages.yaml); dev
// only needs this for its own prerequisites and `update --system`.
type packageManager struct {
	name    string   // as shown to the person ("apt", "dnf", ...)
	bin     string   // what `command -v` looks for
	refresh []string // run before install/upgrade, if any
	install []string
	upgrade []string
	note    string // said before installing, if anything needs saying
}

var linuxPackageManagers = []packageManager{
	{name: "apt", bin: "apt-get",
		refresh: []string{"apt-get", "update", "-qq"},
		install: []string{"env", "DEBIAN_FRONTEND=noninteractive", "apt-get", "install", "-y", "-qq"},
		upgrade: []string{"env", "DEBIAN_FRONTEND=noninteractive", "apt-get", "upgrade", "-y"}},
	{name: "dnf", bin: "dnf",
		install: []string{"dnf", "install", "-y"},
		upgrade: []string{"dnf", "upgrade", "-y"}},
	// Arch has no partial upgrades: installing means syncing the whole system.
	{name: "pacman", bin: "pacman",
		install: []string{"pacman", "-Syu", "--needed", "--noconfirm"},
		upgrade: []string{"pacman", "-Syu", "--noconfirm"},
		note:    "On Arch, installing packages also upgrades the system (pacman -Syu)."},
	{name: "apk", bin: "apk",
		refresh: []string{"apk", "update"},
		install: []string{"apk", "add"},
		upgrade: []string{"apk", "upgrade"}},
}

// packageManager finds this Linux's package manager.
func (r *run) packageManager(ctx context.Context) (packageManager, bool) {
	for _, pm := range linuxPackageManagers {
		if _, err := r.sh(ctx, "command -v "+pm.bin); err == nil {
			return pm, true
		}
	}
	return packageManager{}, false
}

// errNoSudo: something needs root and this machine (or the person) says no sudo.
var errNoSudo = errors.New("needs root, and this machine has no sudo (or was set up with --no-sudo)")

// noSudo: --no-sudo on this run, else what the machine was set up with (dev
// update has no flag for it). Before the first setup there is nothing remembered.
func (r *run) noSudo(ctx context.Context) bool {
	if r.opts.NoSudo != nil {
		return *r.opts.NoSudo
	}
	out, err := r.Runner.Run(ctx, r.chezmoi("execute-template", `{{ dig "nosudo" false . }}`))
	return err == nil && strings.TrimSpace(out) == "true"
}

// rootPrefix is how to run a command as root: directly when root, else sudo.
// It fails in no-sudo mode or when sudo is missing.
func (r *run) rootPrefix(ctx context.Context) ([]string, error) {
	if os.Geteuid() == 0 {
		return nil, nil
	}
	if r.noSudo(ctx) {
		return nil, errNoSudo
	}
	if _, err := r.sh(ctx, "command -v sudo"); err != nil {
		return nil, errNoSudo
	}
	return []string{"sudo"}, nil
}

// asRoot runs pm's refresh (if any), then cmd with args, as root.
func (r *run) asRoot(ctx context.Context, pm packageManager, cmd []string, args ...string) error {
	prefix, err := r.rootPrefix(ctx)
	if err != nil {
		return err
	}
	var steps [][]string
	if pm.refresh != nil {
		steps = append(steps, concat(prefix, pm.refresh))
	}
	steps = append(steps, concat(prefix, cmd, args))
	for _, c := range steps {
		if err := r.attached(ctx, nil, c[0], c[1:]...); err != nil {
			return err
		}
	}
	return nil
}

func concat(parts ...[]string) []string {
	var all []string
	for _, p := range parts {
		all = append(all, p...)
	}
	return all
}

package setup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/neelanjan-mukherji/dev-environment/cli/internal/sys"
)

// UpdateOptions are `dev update`'s flags.
type UpdateOptions struct {
	System bool // also upgrade OS packages
	Yes    bool
	Check  bool // only record how far behind the repo is (for the shell notice)
}

// Update is `dev update`: pull the repo, re-apply (refreshing Zinit, TPM and the
// theme files), upgrade mise tools within their pins, update plugins, and with
// --system upgrade OS packages. It carries on past a failed step and never runs
// on its own: shells only show a notice (see Check).
func Update(ctx context.Context, uo UpdateOptions, d Deps) error {
	r, err := newRun(d, Options{Yes: uo.Yes})
	if err != nil {
		return err
	}
	prependPath(r.bin, filepath.Join(r.home, ".local/share/mise/shims"))
	out, err := r.Runner.Run(ctx, sys.Command("chezmoi", "execute-template", "{{ .chezmoi.workingTree }}"))
	if err != nil {
		return fmt.Errorf("this machine was never set up; run `dev setup` first (%s)", strings.TrimSpace(out))
	}
	repo := strings.TrimSpace(out)
	if uo.Check {
		return r.check(ctx, repo)
	}

	var rows []row
	failed := false
	note := func(name string, err error) {
		if errors.Is(err, errStop) {
			rows = append(rows, row{statusSkipped, name})
			return
		}
		if err != nil {
			failed = true
			rows = append(rows, row{statusFailed, name})
			r.ui.warn(name + ": " + err.Error())
			return
		}
		rows = append(rows, row{statusOK, name})
	}

	r.ui.step("Repository")
	if out, err := r.Runner.Run(ctx, sys.Command("git", "-C", repo, "pull", "--ff-only", "--quiet")); err != nil {
		// Local commits or changes: keep them and apply what is there.
		r.ui.warn("Could not fast-forward " + repo + "; applying it as is. " + strings.TrimSpace(out))
		rows = append(rows, row{statusSkipped, "Repository: not fast-forwarded"})
	} else {
		rows = append(rows, row{statusOK, "Repository"})
	}

	r.ui.step("Dotfiles and install steps")
	skipped, scriptFailed, err := r.apply(ctx, "--refresh-externals=always")
	rows = append(rows, scriptRows(skipped, scriptFailed)...)
	note("Dotfiles and install steps", err)

	r.ui.step("CLI tools (mise upgrade)")
	note("CLI tools", r.attached(ctx, nil, "mise", "upgrade"))

	r.ui.step("Plugins")
	note("zsh plugins", r.attached(ctx, nil, "zsh", "-ic", "zinit update --all --parallel --quiet"))
	note("tmux plugins", r.attached(ctx, nil, filepath.Join(r.home, ".tmux/plugins/tpm/bin/update_plugins"), "all"))
	// Neovim plugins follow the repo's lazy-lock.json (pinned); restore, never bump.
	note("Neovim plugins", r.attached(ctx, nil, "nvim", "--headless", "+Lazy! restore", "+qa"))

	if uo.System {
		r.ui.step("System packages")
		note("System packages", r.upgradeSystem(ctx))
	}

	r.ui.step("Summary")
	r.ui.summary(rows)
	if failed {
		r.ui.warn("Some steps failed; fix the cause and run `dev update` again.")
		return ErrReported
	}
	return os.WriteFile(filepath.Join(r.state, "update-check"), []byte("0\n"), 0o644)
}

// check records how many commits the repo is behind its upstream in
// ~/.local/state/dev/update-check. Shells read it to show a notice; its age
// decides when they check again.
func (r *run) check(ctx context.Context, repo string) error {
	if err := os.MkdirAll(r.state, 0o755); err != nil {
		return err
	}
	path := filepath.Join(r.state, "update-check")
	behind := "0"
	if old, err := os.ReadFile(path); err == nil {
		behind = strings.TrimSpace(string(old))
	}
	if _, err := r.Runner.Run(ctx, sys.Command("git", "-C", repo, "fetch", "--quiet")); err == nil {
		out, err := r.Runner.Run(ctx, sys.Command("git", "-C", repo, "rev-list", "--count", "HEAD..@{upstream}"))
		if n, convErr := strconv.Atoi(strings.TrimSpace(out)); err == nil && convErr == nil {
			behind = strconv.Itoa(n)
		}
	}
	// Written even when offline, so shells don't retry on every start.
	return os.WriteFile(path, []byte(behind+"\n"), 0o644)
}

func (r *run) upgradeSystem(ctx context.Context) error {
	if _, err := r.sh(ctx, "command -v apt-get"); err != nil {
		return errors.New("only apt is supported so far")
	}
	ok, err := r.confirm("Upgrade OS packages with apt (needs sudo)?")
	if err != nil {
		return err
	}
	if !ok {
		return errStop
	}
	root := asRoot()
	if err := r.attached(ctx, nil, root[0], append(root[1:], "apt-get", "update", "-qq")...); err != nil {
		return err
	}
	return r.attached(ctx, nil, root[0], append(root[1:], "env", "DEBIAN_FRONTEND=noninteractive", "apt-get", "upgrade", "-y")...)
}

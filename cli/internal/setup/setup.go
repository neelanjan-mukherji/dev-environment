package setup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/neelanjan-mukherji/dev-environment/cli/internal/sys"
)

// ErrReported means setup already told the person what went wrong.
var ErrReported = errors.New("setup failed")

// Deps are setup's connections to the machine and the person.
type Deps struct {
	Runner   sys.Runner
	Prompter Prompter
	Out      io.Writer
}

type run struct {
	Deps
	o    Options
	ui   ui
	home string
	// state is ~/.local/state/dev; bin is where chezmoi and mise go (~/.local/bin).
	state, bin string
	// chezmoiFlags point chezmoi at a scratch config in a dry run.
	chezmoiFlags []string
	// toolEnv points mise at scratch dirs in a dry run.
	toolEnv []string
	v1      bool
	// settings are the ones configure settled on.
	settings Settings
	// reconfigure: the options say what changes (dev profile/add/remove), so
	// the settings are not reviewed again.
	reconfigure bool
}

// Run is `dev setup`.
func Run(ctx context.Context, o Options, d Deps) error {
	r, err := newRun(d, o)
	if err != nil {
		return err
	}
	home := r.home
	if err := r.preflight(); err != nil {
		return err
	}
	if o.DryRun {
		scratch, err := os.MkdirTemp("", "dev-dry-run-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(scratch)
		r.bin = filepath.Join(scratch, "bin")
		r.chezmoiFlags = []string{"--config", filepath.Join(scratch, "chezmoi.toml"),
			"--persistent-state", filepath.Join(scratch, "state.boltdb"), "--cache", filepath.Join(scratch, "cache")}
		for _, d := range []string{"DATA", "STATE", "CACHE", "CONFIG"} {
			r.toolEnv = append(r.toolEnv, "MISE_"+d+"_DIR="+filepath.Join(scratch, "mise", strings.ToLower(d)))
		}
	}
	prependPath(r.bin, filepath.Join(home, ".local/share/mise/shims"))

	if o.Resume {
		if !r.configured() {
			return errors.New("nothing to resume: this machine was never set up; run `dev setup`")
		}
		return r.applyAndSummarise(ctx)
	}
	if !o.Yes && !d.Prompter.Interactive() {
		return errors.New("nobody to ask: run in a terminal, or pass --yes (with --profile/--name/--email as needed)")
	}
	steps := []func(context.Context) error{r.prerequisites, r.source, r.tools, r.configure}
	if o.DryRun {
		steps = append(steps, r.preview)
	} else {
		steps = append(steps, r.migrateV1, r.review, r.applyAndSummarise, r.identity)
	}
	return r.steps(ctx, steps...)
}

// steps runs steps in order until one fails; a person saying no ends them quietly.
func (r *run) steps(ctx context.Context, steps ...func(context.Context) error) error {
	for _, step := range steps {
		if err := step(ctx); err != nil {
			if errors.Is(err, errStop) {
				return nil
			}
			return err
		}
	}
	return nil
}

func newRun(d Deps, o Options) (*run, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return &run{Deps: d, o: o, ui: newUI(d.Out), home: home, state: stateDir(home), bin: filepath.Join(home, ".local/bin")}, nil
}

// errStop ends setup early without failing (the person said no).
var errStop = errors.New("stopped")

func stateDir(home string) string {
	if x := os.Getenv("XDG_STATE_HOME"); x != "" {
		return filepath.Join(x, "dev")
	}
	return filepath.Join(home, ".local/state/dev")
}

func prependPath(dirs ...string) {
	path := os.Getenv("PATH")
	for _, d := range dirs {
		if !strings.Contains(":"+path+":", ":"+d+":") {
			path = d + ":" + path
		}
	}
	os.Setenv("PATH", path)
}

func (r *run) configured() bool {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		dir = filepath.Join(r.home, ".config")
	}
	_, err := os.Stat(filepath.Join(dir, "chezmoi/chezmoi.toml"))
	return err == nil
}

func (r *run) sh(ctx context.Context, script string, env ...string) (string, error) {
	return r.Runner.Run(ctx, sys.Cmd{Name: "sh", Args: []string{"-c", script}, Env: env})
}

func (r *run) attached(ctx context.Context, env []string, name string, args ...string) error {
	_, err := r.Runner.Run(ctx, sys.Cmd{Name: name, Args: args, Env: env, Attach: true})
	return err
}

func (r *run) chezmoi(args ...string) sys.Cmd {
	return sys.Cmd{Name: "chezmoi", Args: append(args, r.chezmoiFlags...), Env: r.toolEnv}
}

func (r *run) yesEnv() []string {
	if r.o.Yes {
		return []string{"DEVENV_YES=1"}
	}
	return nil
}

// ------------------------------------------------------------------ steps

func (r *run) preflight() error {
	switch runtime.GOOS {
	case "linux":
	case "darwin":
		return errors.New("macOS support arrives in milestone 3")
	default:
		return fmt.Errorf("unsupported OS %s; on Windows, run dev inside WSL", runtime.GOOS)
	}
	if os.Geteuid() == 0 && os.Getenv("DEVENV_ALLOW_ROOT") != "1" {
		return errors.New("run as your normal user, not root (sudo is used only where needed)")
	}
	return nil
}

func (r *run) prerequisites(ctx context.Context) error {
	if _, err := r.sh(ctx, "command -v git && command -v curl"); err == nil {
		return nil
	}
	if r.o.DryRun {
		return errors.New("a dry run needs git and curl; install them first")
	}
	if _, err := r.sh(ctx, "command -v apt-get"); err != nil {
		return errors.New("install git and curl, then re-run")
	}
	r.ui.step("Installing prerequisites (git, curl)")
	if ok, err := r.confirm("Install git, curl and ca-certificates with apt (needs sudo)?"); err != nil || !ok {
		return errors.New("git and curl are required")
	}
	root := asRoot()
	if err := r.attached(ctx, nil, root[0], append(root[1:], "apt-get", "update", "-qq")...); err != nil {
		return err
	}
	return r.attached(ctx, nil, root[0], append(root[1:], "env", "DEBIAN_FRONTEND=noninteractive", "apt-get", "install", "-y", "-qq", "git", "curl", "ca-certificates")...)
}

func asRoot() []string {
	if os.Geteuid() == 0 {
		return []string{"env"}
	}
	return []string{"sudo"}
}

func (r *run) source(ctx context.Context) error {
	if r.o.Source != "" {
		r.ui.info("Source: " + r.o.Source)
		return nil
	}
	dir := os.Getenv("DEVENV_DIR")
	if dir == "" {
		dir = filepath.Join(r.home, "dev-environment")
	}
	url := envOr("DEVENV_REPO_URL", "https://github.com/neelanjan-mukherji/dev-environment.git")
	ref := envOr("DEVENV_REF", "main")
	switch {
	case exists(filepath.Join(dir, ".git")):
		if !r.o.DryRun {
			r.ui.step("Updating " + dir)
			if out, err := r.Runner.Run(ctx, sys.Command("git", "-C", dir, "pull", "--ff-only", "--quiet")); err != nil {
				r.ui.warn("Could not fast-forward " + dir + "; using it as is. " + strings.TrimSpace(out))
			}
		}
	case r.o.DryRun:
		tmp := filepath.Join(filepath.Dir(r.bin), "repo")
		r.ui.step("Cloning " + url + " into a scratch dir")
		if out, err := r.Runner.Run(ctx, sys.Command("git", "clone", "--quiet", "--depth", "1", "--branch", ref, url, tmp)); err != nil {
			return fmt.Errorf("git clone: %w\n%s", err, out)
		}
		dir = tmp
	default:
		r.ui.step("Cloning " + url + " into " + dir)
		if out, err := r.Runner.Run(ctx, sys.Command("git", "clone", "--quiet", "--branch", ref, url, dir)); err != nil {
			return fmt.Errorf("git clone: %w\n%s", err, out)
		}
	}
	r.o.Source = dir
	r.ui.info("Source: " + dir)
	return nil
}

func (r *run) tools(ctx context.Context) error {
	if err := os.MkdirAll(r.bin, 0o755); err != nil {
		return err
	}
	installers := []struct{ name, script string }{
		{"chezmoi", `curl -fsSL https://get.chezmoi.io | sh -s -- -b "$BIN"`},
		{"mise", `curl -fsSL https://mise.run | MISE_INSTALL_PATH="$BIN/mise" sh`},
	}
	for _, t := range installers {
		if t.name == "mise" && r.o.DryRun {
			continue // the preview only needs chezmoi
		}
		if _, err := r.sh(ctx, "command -v "+t.name); err == nil {
			continue
		}
		r.ui.step("Installing " + t.name + " into " + r.bin)
		if out, err := r.sh(ctx, t.script, "BIN="+r.bin); err != nil {
			return fmt.Errorf("installing %s: %w\n%s", t.name, err, out)
		}
	}
	return nil
}

// configure settles profile, name and email, then lets chezmoi write the machine config.
func (r *run) configure(ctx context.Context) error {
	r.ui.step("Machine configuration")
	r.v1 = hasV1Links(r.home)
	if r.v1 {
		r.ui.info("Found a v1 (GNU Stow) setup: its links are replaced first, and the profile defaults to full.")
	}
	tmpl := filepath.Join(r.o.Source, "home/.chezmoi.toml.tmpl")
	if !exists(tmpl) {
		return fmt.Errorf("%s is not a dev-environment checkout (no home/.chezmoi.toml.tmpl)", r.o.Source)
	}
	env := r.o.env()
	if r.v1 && r.o.Profile == "" {
		// A v1 machine had everything that is now the full profile.
		env = append(env, "DEVENV_DEFAULT_PROFILE=full")
	}

	// The template computes the defaults (existing answers, git config, gh); render
	// it without writing anything to learn them.
	c := r.chezmoi("execute-template", "--init", "--source", r.o.Source, "--file", tmpl)
	c.Env = append(c.Env, env...)
	out, err := r.Runner.Run(ctx, c)
	if err != nil {
		return fmt.Errorf("rendering the machine config: %w\n%s", err, out)
	}
	var cfg struct{ Data Settings }
	if _, err := toml.Decode(out, &cfg); err != nil {
		return fmt.Errorf("reading the rendered machine config: %w", err)
	}
	s := cfg.Data

	if !r.o.Yes && !r.reconfigure {
		flagsGiven := r.o.Profile != "" || r.o.Name != "" || r.o.Email != ""
		edit := true
		if r.configured() && !flagsGiven {
			r.ui.settings(s)
			ok, err := r.Prompter.Confirm("Use these settings?")
			if err != nil {
				return err
			}
			edit = !ok
		}
		if edit {
			if s, err = r.Prompter.EditSettings(s); err != nil {
				return err
			}
		}
	}

	o := r.o
	o.Profile, o.Name, o.Email = s.Profile, s.Name, s.Email
	// A space is "blank on purpose" to the template: it neither asks nor falls back to a default.
	if s.Name == "" {
		o.Name = " "
	}
	if s.Email == "" {
		o.Email = " "
	}
	init := r.chezmoi("init", "--source", r.o.Source, "--promptDefaults", "--no-tty")
	init.Env = append(init.Env, o.env()...)
	if r.v1 && r.o.Profile == "" {
		init.Env = append(init.Env, "DEVENV_DEFAULT_PROFILE=full")
	}
	if out, err := r.Runner.Run(ctx, init); err != nil {
		return fmt.Errorf("chezmoi init: %w\n%s", err, out)
	}
	r.settings = s
	r.ui.settings(s)
	return nil
}

// preview is the dry run's ending: what would change, and which steps would run.
func (r *run) preview(ctx context.Context) error {
	r.ui.step("Dry run: what would change (nothing in $HOME is touched)")
	out, _ := r.Runner.Run(ctx, r.chezmoi("diff", "--exclude", "scripts,externals", "--no-pager"))
	r.ui.raw(out)
	r.ui.step("Install steps that would run")
	out, _ = r.Runner.Run(ctx, r.chezmoi("managed", "--include", "scripts"))
	r.ui.list(out)
	return nil
}

// migrateV1 removes v1 Stow links before the preview and backup: both would
// otherwise follow the links into the v1 dotfiles/ tree, and applying would write into it.
func (r *run) migrateV1(ctx context.Context) error {
	if !r.v1 {
		return nil
	}
	tmpl := filepath.Join(r.o.Source, "home/.chezmoiscripts/run_before_00-migrate-v1.sh.tmpl")
	script, err := r.Runner.Run(ctx, r.chezmoi("execute-template", "--file", tmpl))
	if err != nil {
		return fmt.Errorf("rendering the v1 migration: %w\n%s", err, script)
	}
	if err := os.MkdirAll(r.state, 0o755); err != nil {
		return err
	}
	path := filepath.Join(r.state, "migrate-v1.sh")
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		return err
	}
	defer os.Remove(path)
	if err := r.attached(ctx, r.yesEnv(), "bash", path); err != nil {
		return errors.New("the v1 symlinks are still in place, so nothing was applied")
	}
	return nil
}

// review lists the files that will change, backs up their current versions and
// asks before applying.
func (r *run) review(ctx context.Context) error {
	r.ui.step("Files that will change")
	out, _ := r.Runner.Run(ctx, r.chezmoi("status", "--exclude", "scripts,externals"))
	changes := nonEmptyLines(out)
	if len(changes) == 0 {
		r.ui.info("None - dotfiles are up to date.")
		return nil
	}
	r.ui.list(out)
	if dir, n, err := backup(r.home, changes); err != nil {
		return fmt.Errorf("backing up: %w", err)
	} else if n > 0 {
		r.ui.info("Existing versions backed up to " + dir)
	}
	ok, err := r.confirm("Apply these changes and run the install steps?")
	if err != nil {
		return err
	}
	if !ok {
		r.ui.info("Stopped. Review with: chezmoi diff")
		return errStop
	}
	return nil
}

func (r *run) confirm(q string) (bool, error) {
	if r.o.Yes {
		r.ui.info(q + " -> yes (--yes)")
		return true, nil
	}
	return r.Prompter.Confirm(q)
}

func nonEmptyLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

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

// errNotSetUp is the hint for commands that need a configured machine.
var errNotSetUp = errors.New("this machine was never set up; run `dev setup` first")

// errV1 stops anything from applying through v1 Stow links into the old dotfiles/ tree.
var errV1 = errors.New("v1 (GNU Stow) links are still in place; run `dev migrate-v1` (or `dev setup`) first")

// Deps are setup's connections to the machine and the person.
type Deps struct {
	Runner   sys.Runner
	Prompter Prompter
	Out      io.Writer
	OS       string // runtime.GOOS unless a test says otherwise
}

type run struct {
	Deps
	opts Options
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
	// prevConfig is the machine config before configure rewrote it, restored
	// when the person then declines (nil: there was none).
	prevConfig  []byte
	configSaved bool
}

// Run is `dev setup`.
func Run(ctx context.Context, o Options, d Deps) error {
	r, err := newRun(d, o)
	if err != nil {
		return err
	}
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
		prependPath(r.bin)
	}

	if o.Resume {
		if !r.configured() {
			return errNotSetUp
		}
		if hasV1Links(r.home) {
			return errV1
		}
		return r.steps(ctx, r.review, r.applyAndSummarise)
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

// steps runs steps in order until one fails; a person saying no ends them
// quietly. Either way, a machine config written by configure is put back.
func (r *run) steps(ctx context.Context, steps ...func(context.Context) error) error {
	for _, step := range steps {
		if err := step(ctx); err != nil {
			if errors.Is(err, errStop) {
				return r.restoreConfig()
			}
			if rerr := r.restoreConfig(); rerr != nil {
				r.ui.warn("could not restore the machine config: " + rerr.Error())
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
	if d.OS == "" {
		d.OS = runtime.GOOS
	}
	r := &run{Deps: d, opts: o, ui: newUI(d.Out), home: home, state: stateDir(home), bin: filepath.Join(home, ".local/bin")}
	prependPath(r.bin, filepath.Join(home, ".local/share/mise/shims"))
	return r, nil
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

func (r *run) configPath() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		dir = filepath.Join(r.home, ".config")
	}
	return filepath.Join(dir, "chezmoi/chezmoi.toml")
}

func (r *run) configured() bool { return exists(r.configPath()) }

// saveConfig remembers the machine config before configure rewrites it.
func (r *run) saveConfig() {
	if r.opts.DryRun {
		return // a dry run writes a scratch config, never this one
	}
	r.prevConfig, _ = os.ReadFile(r.configPath())
	r.configSaved = true
}

// restoreConfig undoes configure's rewrite after the person declined or a later
// step failed before anything was applied.
func (r *run) restoreConfig() error {
	if !r.configSaved {
		return nil
	}
	r.configSaved = false
	if r.prevConfig == nil {
		err := os.Remove(r.configPath())
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return os.WriteFile(r.configPath(), r.prevConfig, 0o600)
}

// applied marks the point after which the new config is in use and stays.
func (r *run) applied() { r.configSaved = false }

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
	if r.opts.Yes {
		return []string{"DEVENV_YES=1"}
	}
	return nil
}

// confirm asks q; when explain is given the person can ask to see more first.
// Under --yes the answer is yes.
func (r *run) confirm(q Question, explain func()) (bool, error) {
	if r.opts.Yes {
		r.ui.info(q.Text + " -> yes (--yes)")
		return true, nil
	}
	if !r.Prompter.Interactive() {
		r.ui.info(q.Text + " -> no (nobody to ask; --yes answers yes)")
		return false, nil
	}
	if explain == nil {
		q.Explain = ""
	}
	for {
		a, err := r.Prompter.Ask(q)
		if err != nil {
			return false, err
		}
		if a != Explain {
			return a == Yes, nil
		}
		explain()
	}
}

// ------------------------------------------------------------------ steps

func (r *run) preflight() error {
	switch r.OS {
	case "linux", "darwin":
	default:
		return fmt.Errorf("unsupported OS %s; on Windows, run dev inside WSL", r.OS)
	}
	if os.Geteuid() == 0 && os.Getenv("DEVENV_ALLOW_ROOT") != "1" {
		return errors.New("run as your normal user, not root (sudo is used only where needed)")
	}
	return nil
}

func (r *run) prerequisites(ctx context.Context) error {
	if r.OS == "darwin" {
		// macOS ships curl; git is a stub until the Command Line Tools are installed.
		if _, err := r.sh(ctx, "git --version"); err == nil {
			return nil
		}
		r.ui.step("Installing the Command Line Tools (git, compilers)")
		r.ui.command("xcode-select --install")
		if err := r.attached(ctx, nil, "xcode-select", "--install"); err != nil {
			r.ui.warn("xcode-select --install: " + err.Error())
		}
		return errors.New("finish installing the Command Line Tools in the window that opened, then run this again")
	}
	// bash too: the install scripts need it, and Alpine ships without it.
	if _, err := r.sh(ctx, "command -v git && command -v curl && command -v bash"); err == nil {
		return nil
	}
	if r.opts.DryRun {
		return errors.New("a dry run needs git, curl and bash; install them first")
	}
	pm, ok := r.packageManager(ctx)
	if !ok {
		return errors.New("install git, curl and bash, then re-run")
	}
	pkgs := []string{"git", "curl", "ca-certificates", "bash"}
	r.ui.step("Installing prerequisites (" + strings.Join(pkgs, ", ") + ")")
	if pm.note != "" {
		r.ui.info(pm.note)
	}
	if _, err := r.rootPrefix(ctx); err != nil {
		return errors.New("git, curl and bash are needed and there is no sudo here: ask an admin to install them, then re-run")
	}
	if ok, err := r.confirm(Question{Text: "Install them with " + pm.name + " (needs sudo)?"}, nil); err != nil || !ok {
		return errors.New("git, curl and bash are required")
	}
	return r.asRoot(ctx, pm, pm.install, pkgs...)
}

func (r *run) source(ctx context.Context) error {
	if r.opts.Source != "" {
		r.ui.info("Source: " + r.opts.Source)
		return nil
	}
	dir := envOr("DEVENV_DIR", filepath.Join(r.home, "dev-environment"))
	url := envOr("DEVENV_REPO_URL", "https://github.com/neelanjan-mukherji/dev-environment.git")
	ref := envOr("DEVENV_REF", "main")
	switch {
	case exists(filepath.Join(dir, ".git")):
		if !r.opts.DryRun {
			r.ui.step("Updating " + dir)
			if out, err := r.Runner.Run(ctx, sys.Command("git", "-C", dir, "pull", "--ff-only", "--quiet")); err != nil {
				r.ui.warn("Could not fast-forward " + dir + "; using it as is. " + strings.TrimSpace(out))
			}
		}
	case r.opts.DryRun:
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
	r.opts.Source = dir
	r.ui.info("Source: " + dir)
	return nil
}

// tools installs chezmoi and mise with their official install scripts, after
// showing the command and asking.
func (r *run) tools(ctx context.Context) error {
	if err := os.MkdirAll(r.bin, 0o755); err != nil {
		return err
	}
	installers := []struct{ name, script string }{
		{"chezmoi", `curl -fsSL https://get.chezmoi.io | sh -s -- -b "$BIN"`},
		{"mise", `curl -fsSL https://mise.run | MISE_INSTALL_PATH="$BIN/mise" sh`},
	}
	for _, t := range installers {
		if t.name == "mise" && r.opts.DryRun {
			continue // the preview only needs chezmoi
		}
		if _, err := r.sh(ctx, "command -v "+t.name); err == nil {
			continue
		}
		r.ui.step("Installing " + t.name + " into " + r.bin)
		cmd := strings.ReplaceAll(t.script, `"$BIN"`, r.bin)
		ok, err := r.confirm(Question{Text: "Install " + t.name + " with its official install script?", Explain: "Show the command"},
			func() { r.ui.command(cmd) })
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("%s is required; install it yourself (%s), then re-run", t.name, cmd)
		}
		r.ui.command(cmd)
		if out, err := r.sh(ctx, t.script, "BIN="+r.bin); err != nil {
			return fmt.Errorf("installing %s: %w\n%s", t.name, err, out)
		}
	}
	return nil
}

// configure settles profile, name and email, then lets chezmoi write the machine
// config. The previous config is kept so a later "no" can put it back.
func (r *run) configure(ctx context.Context) error {
	r.ui.step("Machine configuration")
	r.v1 = hasV1Links(r.home)
	if r.v1 {
		r.ui.info("Found a v1 (GNU Stow) setup: its links are replaced first, and the profile defaults to full.")
	}
	tmpl := filepath.Join(r.opts.Source, "home/.chezmoi.toml.tmpl")
	if !exists(tmpl) {
		return fmt.Errorf("%s is not a dev-environment checkout (no home/.chezmoi.toml.tmpl)", r.opts.Source)
	}
	var defaults []string
	if r.v1 && r.opts.Profile == "" {
		// A v1 machine had everything that is now the full profile.
		defaults = append(defaults, "DEVENV_DEFAULT_PROFILE=full")
	}

	// The template computes the defaults (existing answers, git config, gh); render
	// it without writing anything to learn them.
	c := r.chezmoi("execute-template", "--init", "--source", r.opts.Source, "--file", tmpl)
	c.Env = append(append(c.Env, r.opts.env()...), defaults...)
	out, err := r.Runner.Run(ctx, c)
	if err != nil {
		return fmt.Errorf("rendering the machine config: %w\n%s", err, out)
	}
	var cfg struct{ Data Settings }
	if _, err := toml.Decode(out, &cfg); err != nil {
		return fmt.Errorf("reading the rendered machine config: %w", err)
	}
	s := cfg.Data

	if !r.opts.Yes && !r.reconfigure {
		flagsGiven := r.opts.Profile != "" || r.opts.Name != "" || r.opts.Email != ""
		edit := true
		if r.configured() && !flagsGiven {
			r.ui.settings(s)
			ok, err := r.confirm(Question{Text: "Use these settings?"}, nil)
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

	o := r.opts
	o.Profile, o.Name, o.Email = s.Profile, orBlank(s.Name), orBlank(s.Email)
	init := r.chezmoi("init", "--source", r.opts.Source, "--promptDefaults", "--no-tty")
	init.Env = append(append(init.Env, o.env()...), defaults...)
	r.saveConfig()
	if out, err := r.Runner.Run(ctx, init); err != nil {
		return fmt.Errorf("chezmoi init: %w\n%s", err, out)
	}
	r.settings = s
	r.ui.settings(s)
	return nil
}

func orBlank(s string) string {
	if s == "" {
		return blank
	}
	return s
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
	tmpl := filepath.Join(r.opts.Source, "home/.chezmoiscripts/run_before_00-migrate-v1.sh.tmpl")
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
// asks before applying (the diff is one answer away).
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
	ok, err := r.confirm(Question{Text: "Apply these changes and run the install steps?", Explain: "Show the diff"}, func() {
		diff, _ := r.Runner.Run(ctx, r.chezmoi("diff", "--exclude", "scripts,externals", "--no-pager"))
		r.ui.raw(diff)
	})
	if err != nil {
		return err
	}
	if !ok {
		r.ui.info("Stopped. Review with: chezmoi diff")
		return errStop
	}
	return nil
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

// Package doctor checks that a machine ended up the way its profile promises.
// The same checks run in CI containers (ci/verify.sh) and on real machines.
package doctor

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/neelanjan-mukherji/dev-environment/cli/internal/sys"
)

// Machine is what this machine was configured with (chezmoi data), and where
// its install steps keep their state.
type Machine struct {
	Profile    string
	Components []string
	Email      string
	Flavor     string // catppuccin flavor, e.g. "mocha"
	OS         string // chezmoi's .chezmoi.os: "linux", "darwin", "windows"
	Arch       string // chezmoi's .chezmoi.arch: "amd64", "arm64"
	Distro     string // the Linux distro's os-release ID ("ubuntu", "alpine"), else empty
	State      string // ~/.local/state/dev
}

// machineTemplate prints one Machine field per line.
const machineTemplate = `{{ .profile }}
{{ includeTemplate "components" . }}
{{ .email }}
{{ .theme.flavor }}
{{ .chezmoi.os }}
{{ .chezmoi.arch }}
{{ dig "osRelease" "id" "" .chezmoi }}
`

// ReadMachine asks chezmoi for this machine's configuration.
func ReadMachine(ctx context.Context, r sys.Runner) (Machine, error) {
	out, err := r.Run(ctx, sys.Command("chezmoi", "execute-template", machineTemplate))
	if err != nil {
		return Machine{}, fmt.Errorf("reading the machine config with chezmoi (has setup run?): %w\n%s", err, out)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	for len(lines) < 7 {
		lines = append(lines, "")
	}
	m := Machine{Profile: lines[0], Email: lines[2], Flavor: lines[3], OS: lines[4], Arch: lines[5], Distro: lines[6], State: stateDir()}
	if lines[1] != "" {
		m.Components = strings.Split(lines[1], ",")
	}
	return m, nil
}

func stateDir() string {
	if x := os.Getenv("XDG_STATE_HOME"); x != "" {
		return filepath.Join(x, "dev")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local/state/dev")
}

// shell is what a check's script runs in.
type shell struct {
	name  string
	flags []string
}

var (
	// Interactive zsh/bash see exactly what the user's shells see (PATH, aliases, plugins).
	interactiveZsh  = shell{"zsh", []string{"-ic"}}
	interactiveBash = shell{"bash", []string{"-ic"}}
	plainBash       = shell{"bash", []string{"-c"}}
	// Windows: Windows PowerShell (always there), without the user's profile.
	powerShell = shell{"powershell", []string{"-NoLogo", "-NoProfile", "-Command"}}
)

// Check is one assertion, run as a shell snippet.
type Check struct {
	Name   string
	Shell  shell
	Script string
	// Skipped reports whether the install step behind this check was skipped
	// on purpose (no sudo, declined); then a failure is reported as a skip.
	Skipped func() bool
}

func zsh(name, script string) Check  { return Check{Name: name, Shell: interactiveZsh, Script: script} }
func bash(name, script string) Check { return Check{Name: name, Shell: plainBash, Script: script} }

// loginShellIsZsh reads the login shell the way the install step does
// (getent on Linux, Directory Services on macOS).
const loginShellIsZsh = `u="$(id -un)"; if command -v getent >/dev/null; then getent passwd "$u" | cut -d: -f7; else dscl . -read "/Users/$u" UserShell | awk '{print $2}'; fi | grep -q zsh`

// shellQuote quotes s for a POSIX shell.
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// skippedStep reports whether a step was recorded as skipped: a line in
// skipped.log starting with prefix, or the step's "declined" marker.
func skippedStep(state, prefix, marker string) func() bool {
	return func() bool {
		if _, err := os.Stat(filepath.Join(state, marker)); err == nil {
			return true
		}
		log, _ := os.ReadFile(filepath.Join(state, "skipped.log"))
		for _, l := range strings.Split(string(log), "\n") {
			if _, entry, ok := strings.Cut(l, "\t"); ok && strings.HasPrefix(strings.ToLower(entry), prefix) {
				return true
			}
		}
		return false
	}
}

var coreTools = []string{"rg", "fd", "bat", "eza", "fzf", "zoxide", "starship", "jq", "jless", "lazygit", "yazi", "duf", "gh", "nvim", "tmux", "btop"}

// componentTools lists the commands each component puts on PATH. For mise
// components the first one is also how the doctor tells that mise does not
// manage a disabled component; Docker is a system package.
var componentTools = []struct {
	name  string
	tools []string
	mise  bool
}{
	{"devtools", []string{"kondo"}, true},
	{"node", []string{"node", "npm"}, true},
	{"python", []string{"python", "uv"}, true},
	{"go", []string{"go"}, true},
	{"rust", []string{"cargo"}, true},
	{"docker", []string{"docker"}, false},
	{"java", []string{"java", "mvn", "gradle", "spring"}, true},
	{"kubernetes", []string{"kubectl", "helm", "k9s"}, true},
	{"cloud", []string{"aws", "gcloud", "az"}, true},
}

// Checks returns every check that applies to m.
func Checks(m Machine) []Check {
	if m.OS == "windows" {
		return windowsChecks(m)
	}
	var cs []Check
	for _, t := range coreTools {
		// jless has no Linux arm64 build (Alpine gets it from apk); the mise config leaves it out.
		if t == "jless" && m.OS == "linux" && m.Arch == "arm64" && m.Distro != "alpine" {
			continue
		}
		cs = append(cs, zsh("zsh has "+t, "command -v "+t))
	}
	cs = append(cs,
		zsh("neovim is pinned to 0.11", `nvim --version | head -1 | grep -q "v0.11"`),
		zsh("zinit plugins load", `whence -w zinit | grep -q function`),
		bash("tmux plugin manager present", `test -x "$HOME/.tmux/plugins/tpm/tpm"`),
		bash("neovim plugins restored", `test -d "$HOME/.local/share/nvim/lazy/telescope.nvim"`),
		zsh("bat theme compiled", "bat --list-themes | grep -q "+shellQuote("Catppuccin "+title(m.Flavor))),
		Check{Name: "bash fallback has tools + aliases", Shell: interactiveBash, Script: `command -v starship && command -v rg && alias gs`},
		Check{Name: "login shell is zsh", Shell: plainBash, Script: loginShellIsZsh,
			Skipped: skippedStep(m.State, "login shell", "login-shell.declined")},
	)
	if m.Email != "" {
		cs = append(cs, bash("git identity from .gitconfig.local", `[ "$(git config user.email)" = `+shellQuote(m.Email)+` ]`))
	}

	on := func(c string) bool { return slices.Contains(m.Components, c) }
	// Docker is skipped, not failed, where it can't be installed (no sudo, Alpine).
	dockerSkipped := skippedStep(m.State, "docker", "docker.skipped")
	for _, c := range componentTools {
		if on(c.name) {
			for _, t := range c.tools {
				check := zsh("zsh has "+t, "command -v "+t)
				if c.name == "docker" {
					check.Skipped = dockerSkipped
				}
				cs = append(cs, check)
			}
		} else if c.mise {
			cs = append(cs, zsh("mise does not manage "+c.name, "! mise which "+c.tools[0]))
		}
	}
	if m.OS == "darwin" {
		cs = append(cs,
			bash("WezTerm installed", `test -d /Applications/WezTerm.app || test -d "$HOME/Applications/WezTerm.app" || command -v wezterm`),
			bash("JetBrainsMono Nerd Font installed", `ls "$HOME/Library/Fonts" /Library/Fonts 2>/dev/null | grep -qi 'JetBrainsMonoNerdFont'`),
		)
		if on("docker") {
			cs = append(cs, zsh("zsh has colima", "command -v colima"))
		}
	}
	if on("node") {
		cs = append(cs, zsh("node is the pinned major", `node --version | grep -q "^v26\."`))
	}
	if on("docker") {
		compose := bash("docker compose plugin", `docker compose version`)
		compose.Skipped = dockerSkipped
		cs = append(cs, compose)
	}
	if on("java") {
		cs = append(cs, zsh("default JDK is Corretto 21", `java -version 2>&1 | grep -q "Corretto-21"`))
		for _, v := range []string{"8", "17", "25"} {
			cs = append(cs, zsh("Corretto "+v+" installed", "mise where java@corretto-"+v))
		}
	}

	cs = append(cs,
		// Scripts are excluded: run_after_ steps always show as pending (they re-check each apply).
		bash("dotfiles in sync", `out="$(chezmoi status --exclude externals,scripts)" && { [ -z "$out" ] || { echo "$out"; chezmoi diff --exclude externals,scripts --no-pager | head -40; false; }; }`),
		bash("no step failed", `f="${XDG_STATE_HOME:-$HOME/.local/state}/dev/failed.log"; [ ! -s "$f" ] || { cat "$f"; false; }`),
	)
	return cs
}

// parallel bounds how many checks run at once (each starts an interactive shell).
const parallel = 8

// Run runs the checks concurrently, reports each one to w in order and returns
// how many failed.
func Run(ctx context.Context, r sys.Runner, checks []Check, w io.Writer) (failed int) {
	type result struct {
		out string
		err error
	}
	results := make([]chan result, len(checks))
	sem := make(chan struct{}, parallel)
	for i, c := range checks {
		results[i] = make(chan result, 1)
		go func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			out, err := r.Run(ctx, sys.Command(c.Shell.name, append(slices.Clone(c.Shell.flags), c.Script)...))
			results[i] <- result{out, err}
		}()
	}

	style := lipgloss.NewRenderer(w)
	okStyle := style.NewStyle().Foreground(lipgloss.Color("2"))
	failStyle := style.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)
	skipStyle := style.NewStyle().Foreground(lipgloss.Color("3"))
	skipped := 0
	for i, c := range checks {
		res := <-results[i]
		if res.err == nil {
			fmt.Fprintf(w, "%s   %s\n", okStyle.Render("ok"), c.Name)
			continue
		}
		if c.Skipped != nil && c.Skipped() {
			skipped++
			fmt.Fprintf(w, "%s %s (the install step was skipped; see skipped.log)\n", skipStyle.Render("skip"), c.Name)
			continue
		}
		failed++
		fmt.Fprintf(w, "%s %s\n", failStyle.Render("FAIL"), c.Name)
		for _, l := range lastLines(res.out, 40) {
			fmt.Fprintf(w, "    %s\n", l)
		}
	}
	fmt.Fprintf(w, "---- %d passed, %d failed", len(checks)-failed-skipped, failed)
	if skipped > 0 {
		fmt.Fprintf(w, ", %d skipped", skipped)
	}
	fmt.Fprintln(w)
	return failed
}

// title is "Mocha" for "mocha".
func title(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func lastLines(s string, n int) []string {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines
}

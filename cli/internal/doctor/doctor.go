// Package doctor checks that a machine ended up the way its profile promises.
// The same checks run in CI containers (ci/verify.sh) and on real machines.
package doctor

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/neelanjan-mukherji/dev-environment/cli/internal/sys"
)

// Machine is what this machine was configured with (chezmoi data).
type Machine struct {
	Profile    string
	Components []string
	Email      string
}

// machineTemplate prints one Machine field per line.
const machineTemplate = `{{ .profile }}
{{ includeTemplate "components" . }}
{{ .email }}
`

// ReadMachine asks chezmoi for this machine's configuration.
func ReadMachine(ctx context.Context, r sys.Runner) (Machine, error) {
	out, err := r.Run(ctx, sys.Command("chezmoi", "execute-template", machineTemplate))
	if err != nil {
		return Machine{}, fmt.Errorf("reading the machine config with chezmoi (has setup run?): %w\n%s", err, out)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	for len(lines) < 3 {
		lines = append(lines, "")
	}
	m := Machine{Profile: lines[0], Email: lines[2]}
	if lines[1] != "" {
		m.Components = strings.Split(lines[1], ",")
	}
	return m, nil
}

// Check is one assertion, run as a shell snippet. Interactive zsh/bash checks see
// exactly what the user's shells see (PATH, aliases, plugins).
type Check struct {
	Name   string
	Shell  string // "zsh -i", "bash -i" or "bash"
	Script string
}

func (c Check) command() (string, []string) {
	switch c.Shell {
	case "zsh -i":
		return "zsh", []string{"-ic", c.Script}
	case "bash -i":
		return "bash", []string{"-ic", c.Script}
	default:
		return "bash", []string{"-c", c.Script}
	}
}

func zsh(name, script string) Check  { return Check{name, "zsh -i", script} }
func bash(name, script string) Check { return Check{name, "bash", script} }

var coreTools = []string{"rg", "fd", "bat", "eza", "fzf", "zoxide", "starship", "jq", "jless", "lazygit", "yazi", "duf", "gh", "nvim", "tmux", "btop"}

// componentTools lists the commands each component puts on PATH. The first one
// is also how the doctor tells that mise does not manage a disabled component.
var componentTools = []struct {
	name  string
	tools []string
}{
	{"devtools", []string{"kondo"}},
	{"node", []string{"node", "npm"}},
	{"python", []string{"python", "uv"}},
	{"go", []string{"go"}},
	{"rust", []string{"cargo"}},
	{"docker", []string{"docker"}},
	{"java", []string{"java", "mvn", "gradle", "spring"}},
	{"kubernetes", []string{"kubectl", "helm", "k9s"}},
	{"cloud", []string{"aws", "gcloud", "az"}},
}

// Checks returns every check that applies to m.
func Checks(m Machine) []Check {
	var cs []Check
	for _, t := range coreTools {
		cs = append(cs, zsh("zsh has "+t, "command -v "+t))
	}
	cs = append(cs,
		zsh("neovim is pinned to 0.11", `nvim --version | head -1 | grep -q "v0.11"`),
		zsh("zinit plugins load", `whence -w zinit | grep -q function`),
		bash("tmux plugin manager present", `test -x "$HOME/.tmux/plugins/tpm/tpm"`),
		bash("neovim plugins restored", `test -d "$HOME/.local/share/nvim/lazy/telescope.nvim"`),
		zsh("bat theme compiled", `bat --list-themes | grep -q "Catppuccin Mocha"`),
		Check{"bash fallback has tools + aliases", "bash -i", `command -v starship && command -v rg && alias gs`},
		bash("login shell is zsh", `getent passwd "$(id -un)" | cut -d: -f7 | grep -q zsh`),
	)
	if m.Email != "" {
		cs = append(cs, bash("git identity from .gitconfig.local", fmt.Sprintf(`[ "$(git config user.email)" = %q ]`, m.Email)))
	}

	on := func(c string) bool { return slices.Contains(m.Components, c) }
	for _, c := range componentTools {
		if on(c.name) {
			for _, t := range c.tools {
				cs = append(cs, zsh("zsh has "+t, "command -v "+t))
			}
		} else if c.name != "docker" { // Docker is a system package, not a mise tool.
			cs = append(cs, zsh("mise does not manage "+c.name, "! mise which "+c.tools[0]))
		}
	}
	if on("node") {
		cs = append(cs, zsh("node is the pinned major", `node --version | grep -q "^v26\."`))
	}
	if on("docker") {
		cs = append(cs, bash("docker compose plugin", `docker compose version`))
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
			name, args := c.command()
			out, err := r.Run(ctx, sys.Command(name, args...))
			results[i] <- result{out, err}
		}()
	}

	style := lipgloss.NewRenderer(w)
	okStyle := style.NewStyle().Foreground(lipgloss.Color("2"))
	failStyle := style.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)
	for i, c := range checks {
		res := <-results[i]
		if res.err == nil {
			fmt.Fprintf(w, "%s   %s\n", okStyle.Render("ok"), c.Name)
			continue
		}
		failed++
		fmt.Fprintf(w, "%s %s\n", failStyle.Render("FAIL"), c.Name)
		for _, l := range lastLines(res.out, 40) {
			fmt.Fprintf(w, "    %s\n", l)
		}
	}
	fmt.Fprintf(w, "---- %d passed, %d failed\n", len(checks)-failed, failed)
	return failed
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

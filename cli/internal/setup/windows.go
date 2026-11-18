package setup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/neelanjan-mukherji/dev-environment/cli/internal/sys"
)

// The Windows host. winget installs the tools (packages.yaml's winget list, via
// the chezmoi scripts); dev only needs it for git and chezmoi, before chezmoi
// can run. The Linux side is WSL, set up by the same install.sh as any Linux.

// linuxInstall is the one-liner that sets up the Linux side from the given
// branch (the host checkout's, so both sides run the same version), with args.
func linuxInstall(ref string, args ...string) string {
	cmd := "curl -fsSL https://raw.githubusercontent.com/neelanjan-mukherji/dev-environment/" + ref +
		"/install.sh | DEVENV_REF=" + shQuote(ref) + " sh"
	if len(args) > 0 {
		var quoted []string
		for _, a := range args {
			quoted = append(quoted, shQuote(a))
		}
		cmd += " -s -- " + strings.Join(quoted, " ")
	}
	return cmd
}

// shQuote quotes s for sh unless it is plainly safe.
func shQuote(s string) string {
	if s != "" && strings.Trim(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789@._/+-=:,") == "" {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// sourceRef is the branch of the host's checkout ("main" when detached or unknown).
func (r *run) sourceRef(ctx context.Context) string {
	out, err := r.Runner.Run(ctx, sys.Command("git", "-C", r.opts.Source, "rev-parse", "--abbrev-ref", "HEAD"))
	if ref := strings.TrimSpace(out); err == nil && ref != "" && ref != "HEAD" {
		return ref
	}
	return "main"
}

var errNoWinget = errors.New("winget is missing: install App Installer from the Microsoft Store (https://aka.ms/getwinget), then re-run")

// has reports whether a command is on PATH.
func (r *run) has(ctx context.Context, name string) bool {
	if r.OS == "windows" {
		_, err := r.Runner.Run(ctx, sys.Command("where.exe", name))
		return err == nil
	}
	_, err := r.sh(ctx, "command -v "+name)
	return err == nil
}

// wingetInstall installs one package, showing the command.
func (r *run) wingetInstall(ctx context.Context, id string) error {
	if !r.has(ctx, "winget") {
		return errNoWinget
	}
	args := []string{"install", "--id", id, "--exact", "--source", "winget",
		"--accept-package-agreements", "--accept-source-agreements", "--silent"}
	r.ui.command("winget " + strings.Join(args, " "))
	if err := r.attached(ctx, nil, "winget", args...); err != nil {
		return err
	}
	r.refreshPath(ctx)
	return nil
}

// refreshPath picks up what installers added to PATH: a running process keeps
// the PATH it started with, so it is read back from the registry.
func (r *run) refreshPath(ctx context.Context) {
	out, err := r.Runner.Run(ctx, sys.Command("powershell", "-NoProfile", "-Command",
		"[Environment]::GetEnvironmentVariable('Path','Machine') + ';' + [Environment]::GetEnvironmentVariable('Path','User')"))
	if err != nil {
		return
	}
	var dirs []string
	for _, d := range strings.Split(strings.TrimSpace(out), ";") {
		if d = strings.TrimSpace(d); d != "" {
			dirs = append(dirs, d)
		}
	}
	prependPath(dirs...)
}

// prerequisitesWindows: git (for the repo and Neovim's plugins), from winget.
func (r *run) prerequisitesWindows(ctx context.Context) error {
	// winget installs everything on the host (packages.yaml), so it comes first.
	if !r.has(ctx, "winget") {
		return errNoWinget
	}
	if r.has(ctx, "git") {
		return nil
	}
	if r.opts.DryRun {
		return errors.New("a dry run needs git; install it first (winget install Git.Git)")
	}
	r.ui.step("Installing the prerequisite (git)")
	if ok, err := r.confirm(Question{Text: "Install Git for Windows with winget?"}, nil); err != nil || !ok {
		return errors.New("git is required")
	}
	return r.wingetInstall(ctx, "Git.Git")
}

// toolsWindows: chezmoi from winget (mise stays on the Linux side).
func (r *run) toolsWindows(ctx context.Context) error {
	if r.has(ctx, "chezmoi") {
		return nil
	}
	r.ui.step("Installing chezmoi")
	if ok, err := r.confirm(Question{Text: "Install chezmoi with winget?"}, nil); err != nil || !ok {
		return errors.New("chezmoi is required; install it yourself (winget install twpayne.chezmoi), then re-run")
	}
	return r.wingetInstall(ctx, "twpayne.chezmoi")
}

// wsl offers the Linux side: installing WSL when there is none, else running
// the Linux setup inside the distro. Under --yes only with --wsl (installing
// WSL needs a restart).
func (r *run) wsl(ctx context.Context) error {
	if r.OS != "windows" {
		return nil
	}
	r.ui.step("WSL (the Linux side: zsh, Docker, languages)")
	if r.opts.Yes && !r.opts.WSL {
		r.ui.info("Left alone under --yes; pass --wsl to install WSL or set up its Linux side.")
		return r.recordSkip("wsl: left alone under --yes (no --wsl)")
	}
	ref := r.sourceRef(ctx)
	out, err := r.Runner.Run(ctx, sys.Cmd{Name: "wsl.exe", Args: []string{"--list", "--verbose"}, Env: []string{"WSL_UTF8=1"}})
	distro := defaultDistro(out)
	if err != nil || distro == "" {
		ok, err := r.confirm(Question{Text: "Install WSL with Ubuntu (asks for admin rights; needs a restart)?"}, nil)
		if err != nil {
			return err
		}
		if !ok {
			r.ui.info("Later: wsl --install -d Ubuntu, then inside Ubuntu: " + linuxInstall(ref))
			return r.recordSkip("wsl: not installed (declined)")
		}
		r.ui.command("wsl --install -d Ubuntu")
		if err := r.attached(ctx, nil, "wsl.exe", "--install", "-d", "Ubuntu"); err != nil {
			return err
		}
		r.ui.info("Restart Windows, open Ubuntu from the Start menu to create your Linux user, then run there:")
		r.ui.command(linuxInstall(ref))
		return r.recordSkip("wsl: installed; finish after a restart (" + linuxInstall(ref) + " inside Ubuntu)")
	}
	ok, err := r.confirm(Question{Text: "Set up the Linux side in WSL (" + distro + ") too?"}, nil)
	if err != nil {
		return err
	}
	if !ok {
		r.ui.info("Later, inside WSL: " + linuxInstall(ref))
		return r.recordSkip("wsl: Linux side not set up (declined)")
	}
	// The answers given here carry over, so WSL doesn't ask again.
	var args []string
	for _, kv := range [][2]string{{"--profile", r.settings.Profile}, {"--name", r.settings.Name}, {"--email", r.settings.Email}} {
		if kv[1] != "" {
			args = append(args, kv[0], kv[1])
		}
	}
	if r.opts.Yes {
		args = append(args, "--yes")
	}
	cmd := linuxInstall(ref, args...)
	r.ui.command("wsl -d " + distro + " --exec sh -c '" + cmd + "'")
	// --exec: the arguments reach sh as they are, not through the login shell's parsing.
	return r.attached(ctx, nil, "wsl.exe", "-d", distro, "--exec", "sh", "-c", cmd)
}

// defaultDistro: the distro `wsl --list --verbose` marks with "*" (the default),
// else the first one listed; never Docker Desktop's internal ones. The header
// row is localized, so rows are told apart by position only.
func defaultDistro(out string) string {
	lines := strings.Split(strings.ReplaceAll(out, "\x00", ""), "\n")
	first := ""
	for i, l := range lines {
		l = strings.TrimSpace(l)
		if i == 0 || l == "" {
			continue // header
		}
		starred := strings.HasPrefix(l, "*")
		fields := strings.Fields(strings.TrimPrefix(l, "*"))
		if len(fields) == 0 || strings.HasPrefix(fields[0], "docker-desktop") {
			continue
		}
		if starred {
			return fields[0]
		}
		if first == "" {
			first = fields[0]
		}
	}
	return first
}

// recordSkip adds a line to skipped.log, like the install scripts' record_skip.
func (r *run) recordSkip(what string) error {
	if err := os.MkdirAll(r.state, 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(r.state, "skipped.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(time.Now().Format("2006-01-02 15:04:05") + "\t" + what + "\n")
	return err
}

// pathSep is the PATH list separator of the OS dev runs on.
var pathSep = string(os.PathListSeparator)

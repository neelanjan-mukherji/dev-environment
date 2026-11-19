# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

Personal workstation setup meant to run on every OS. The v2 redesign lives on branch `v2`; `docs/v2-design.md` is the agreed design and milestone plan (read it before structural changes). `v1.0.0` tags the old Ubuntu/WSL-only Stow setup. Milestones 1 (Ubuntu/Debian incl. WSL), 2 (the `dev` CLI), 3 (macOS), 4 (Fedora, Arch, Alpine minimal, no-sudo, arm64), 5 (the Windows host) and 6 (the optional `desktop` component) are done.

## Commands

```bash
make check                    # shellcheck + go vet/test (cli/) + ci/test-render.sh (fast, no Docker, never touches $HOME)
make build                    # cli/dist/dev (the dev binary; `cd cli && go test ./... -run <Name>` for one Go test)
make test FILTER=<substring>  # run matching render tests only
make test-containers          # ci/run-containers.sh (other distros: pass images, e.g. fedora:43 archlinux:latest alpine:3.22; NOSUDO=1 for a no-sudo machine): install.sh --yes (host-built dev via DEVENV_DEV_BIN) + ci/verify.sh in clean ubuntu/debian containers ("full" runs as a v1 migration via ci/seed-v1.sh)
make verify                   # build, then ci/verify.sh (dev doctor + second-install no-op) against this machine
BASES=debian:13 PROFILES=full make test-containers
cli/dist/dev setup --dry-run   # renders against the real $HOME with scratch config/state; changes nothing
```

Render tests need `chezmoi`, `shellcheck`, `zsh` and `script` (util-linux) on PATH; the CLI needs Go (pinned in `cli/go.mod`).

## Architecture

- `.chezmoiroot` = `home`, so `home/` is the chezmoi source state. The repo root holds `install.sh` (bootstrap: fetches or builds `dev`, then `dev setup`), `cli/`, `ci/`, and `docs/`. chezmoi's `sourceDir` is the clone itself (`~/dev-environment`).
- **Machine config.** `home/.chezmoi.toml.tmpl` asks once for `profile`, `name` and `email`. Each value can be overridden with an env var: `DEVENV_PROFILE`, `DEVENV_NAME`, `DEVENV_EMAIL`, `DEVENV_ADD`, `DEVENV_REMOVE`, `DEVENV_THEME`, `DEVENV_NO_SUDO` (1/0; persisted as `nosudo`, which lib.sh's `can_sudo` honours). Values already in the existing config are carried over on re-init. Use the env vars rather than `--promptChoice`/`--promptString`, whose keys are the prompt *text*. `dev setup` flags map onto these env vars. A single space means "blank on purpose" (name/email: no prompt, no default; add/remove: empty list); that's how `dev` clears a value.
- **Components.** `home/.chezmoidata/profiles.yaml` maps each profile to its components. `home/.chezmoitemplates/components` computes the effective list (profile + add − remove, plus each component's `$requires`, e.g. cloud → python); templates use it via `includeTemplate "components" . | splitList ","`. Any new component must be added to `$known` in the config template as well.
  - **`desktop`** (VS Code, its extensions and `settings.json`, macOS/Windows OS tweaks) is in no profile and has no `$requires`. Its extension and tweak lists live in `home/.chezmoidata/desktop.yaml`, read by the install scripts and by the doctor (`machineTemplate` passes the data as JSON). `settings.json` is a `modify_` file at each OS's own path (it re-sorts the keys: `toPrettyJson` over a map; accepted), kept out by `.chezmoiignore` unless `desktop` is on, and inside WSL (the host owns VS Code; `.chezmoiignore` can't detect at runtime, so it reads `.chezmoi.kernel.osrelease`, as the doctor's `machineTemplate` does; render tests fake it with `RENDER_KERNEL`). `dev remove desktop` uninstalls and reverts nothing.
- **Packages.** mise installs every tool and language (`home/private_dot_config/mise/config.toml.tmpl`). The OS package manager installs only the base packages in `home/.chezmoidata/packages.yaml`. Policy: CLI tools use `latest`; toolchains and Neovim are pinned.
- **Install scripts.** `home/.chezmoiscripts/` holds the install steps; all of them include `.chezmoitemplates/lib.sh`, which provides `step`, `run`, `as_root`, `confirm`, `can_sudo`, `record_skip`, and `package_manager`/`pkg_installed`/`pkg_install` (apt, dnf, pacman, apk).
  - **Conventions:** echo every system-changing command (`run` echoes to stderr, so redirecting output never hides it); `confirm` before risky actions (`DEVENV_YES=1` answers yes); when something is impossible (no sudo, unsupported distro), call `record_skip` and exit 0; exit non-zero only on real failure: lib.sh logs it to `failed.log` under the current `step` name, and chezmoi retries the script on the next apply.
  - **Exception:** `run_before_00-migrate-v1` fails when declined, since applying over v1 directory links would write into the old tree. `dev setup` runs it before its preview/backup and stops if it fails.
  - **Steps that must retry after a skip** (login shell, retiring v1 tools, VS Code on Linux and its extensions, the OS tweaks) are `run_after_` scripts that stay silent when there is nothing to do; verify.sh's no-op check depends on that silence. A declined prompt is remembered in `~/.local/state/dev/`.
  - **Gating:** `run_onchange_` scripts embed a hash of their inputs, which is what makes them re-run. A script that renders to empty is skipped, and that's how OS/component gating works.
- **`dev` CLI** (`cli/`, Go + cobra/huh/lipgloss). `internal/app.Main` is the whole CLI behind `Deps` (a `sys.Runner` for every external command, a `setup.Prompter` for the person); tests in `internal/app` drive `Main` against fakes. The chezmoi scripts stay the install engine; `dev` orchestrates them:
  - `setup` (`internal/setup`): prerequisites → source → chezmoi/mise → configure (renders `.chezmoi.toml.tmpl` with `execute-template --init` to learn the defaults, huh form, then `chezmoi init` with `DEVENV_*`) → v1 migration → review/backup → `apply --keep-going` → summary → identity (SSH key, gh). `--resume` only re-applies. Each apply moves `failed.log` to `failed.prev.log` first, so `failed.log` is the latest run only.
  - `update` re-applies with `--refresh-externals=always`, runs `mise upgrade`, plugin updates and `Lazy! restore` (the lock stays the repo's). `update --check` writes `~/.local/state/dev/update-check`, which `~/.config/shell/update-notice.sh` reads.
  - `profile`/`add`/`remove` re-init with merged overrides; non-component names are mise tools in `~/.config/mise/conf.d/local.toml`. `setup.KnownComponents` must match `$known` in the config template (a render test checks), and `setup.OptionalComponents` (what the setup form offers on top of the profile) must be the known components in no profile of `profiles.yaml` (a Go test checks).
  - `doctor` (`internal/doctor`) holds the machine checks, derived from the effective components; `ci/verify.sh` runs it and adds only the post-install checks (v1 migration, second install is a no-op). A check whose install step recorded a skip reports `skip`: `skippedStep` matches a marker file or the leading words of a `skipped.log` entry, so a skip message's opening words (`vscode:` vs `vscode extensions:`, `macos tweaks:`) are an interface; change them in the script and the doctor together. A new step header must also join the no-op regex in `ci/verify.sh` / `ci/verify.ps1`.
  - Releases: `.goreleaser.yaml` + `.github/workflows/release.yml` on `v*` tags; asset names are what `install.sh` downloads.
- **Externals.** `home/.chezmoiexternal.toml.tmpl` fetches Zinit, TPM and the Catppuccin theme files, so shell startup never touches the network. `.zshrc` must keep working when Zinit or tools are missing; the render tests assert that.
- **PATH** is defined once in `~/.config/shell/path.sh`, sourced by `.zshenv`, `.profile`, `.bashrc` and lib.sh.
- **Shells.** zsh and the bash fallback share `~/.config/shell/{env.sh,aliases.sh,functions.sh}`; those files must stay valid in both shells. zsh-only syntax goes in `~/.config/zsh/aliases.zsh`. Integrations stay guarded with `command -v`.
- **Theme.** `home/.chezmoidata/catppuccin.yaml` is generated from catppuccin/palette (don't hand-edit it). Templates read `(index .catppuccin .theme.flavor).colors`.
- **Platform differences.** Detect at runtime where possible (`clipcopy`, `has("wsl")` in nvim, the opener in `aliases.zsh`, `login_shell` in lib.sh). Template on `.chezmoi.os` only when runtime detection isn't possible. OS-specific install steps are separate scripts gated on `.chezmoi.os` (`*-darwin.sh.tmpl` beside the apt ones); OS-specific targets go in `home/.chezmoiignore`. In Go, `Deps.OS` replaces `runtime.GOOS` so tests can play macOS (`runOn`).
- **Linux distros.** Install scripts pick the package manager at runtime (`package_manager` in lib.sh); `packages.yaml` has one list per manager, and they should stay equivalent. Templates use `.chezmoi.osRelease.id` (via `dig`) only where runtime detection can't work, e.g. Alpine's mise config. `chezmoi init` ignores `--override-data`, so render tests can fake a distro for `apply`/`execute-template` but not for the config template; the container runs cover that. In Go, `setup/packages.go` holds the same managers for dev's prerequisites (git, curl, bash) and `update --system`.
- **Windows host.** `dev` runs natively (`install.ps1` → `dev.exe setup`); `Deps.OS == "windows"` switches setup to winget prerequisites, no mise, and a final WSL step (`setup/windows.go`), and the doctor to PowerShell checks (`doctor/windows.go`). `.chezmoiignore` drops `.sh` scripts and the Unix shell files on Windows and `.ps1` scripts, `AppData` and `.config/powershell` elsewhere; render tests list the scripts per OS with `chezmoi managed`. Windows install scripts include `.chezmoitemplates/lib.ps1` (`Step`, `Info`, `Invoke-Logged`, `Confirm-Step`, `Add-Skip`, failed.log via `trap`) and must run under Windows PowerShell 5.1 (no `` `e `` escape, no ternary or `??` operators). Build paths with `Join-Path`, not `\` strings, so the pwsh render tests (Linux pwsh, CI) exercise them. The config template's `sh` lookups use Git for Windows' `sh.exe`. With `desktop`, VS Code belongs to the host (winget, plus the WSL extension); the Linux side under WSL records a skip for VS Code and its extensions. Registry tweaks go through `reg.exe` and gate on the build number (the classic context menu is Windows 11, build 22000+; the CI runner is Windows Server, so it never applies there).
- **macOS.** On a fresh Mac every script of the first apply runs under `/bin/bash` 3.2 (chezmoi's PATH predates Homebrew): no bash 4 features, and no empty-array expansion under `set -u`. The Homebrew step is `run_before_` (retried after a skip, silent when all is installed). BSD tools: no `sed -i`, `\x1b` or `getent`. The VS Code cask has `auto_updates`, so `brew upgrade` (and `dev update --system`) skips it without `--greedy`; VS Code updates itself. Render tests cover darwin with `RENDER_OS=darwin render …` and `render_scripts <profile> darwin` (chezmoi `--override-data`); there is no local macOS run, so the `macos` workflow (`.github/workflows/macos.yml`, started by hand: `gh workflow run macos.yml --ref v2`, since GitHub's macOS runners are often unavailable) is the real check.

## Gotchas

- Files under `$HOME` are rendered copies, not symlinks. Edit `home/` (or use `chezmoi edit`), then run `chezmoi apply`. `git config --global` writes to `~/.gitconfig`, and the next apply reverts it; identity belongs in `~/.gitconfig.local`.
- `modify_` files (e.g. `btop.conf`, which btop rewrites itself) must **not** have a `.tmpl` suffix; they use the `chezmoi:modify-template` marker.
- Interactive zsh must never stop to ask anything: GitHub's runner image has world-writable completion dirs, and a plain `compinit` (ours, or Debian/Ubuntu's global one, skipped via `skip_global_compinit` in `.zshenv`) then waits for an answer forever. Render tests give interactive shells a 60s `timeout`; on CI they print each test name before running it.
- `.gitattributes` forces LF: with Git for Windows' default `core.autocrlf=true` the source (and so every rendered file) would be CRLF, and lazy.nvim rewrites `lazy-lock.json` in LF, which the doctor's "dotfiles in sync" check then fails.
- jless has no Linux arm64 build: the mise config leaves it out there and the doctor skips it (it reads `.chezmoi.arch` and the os-release ID).
- Render-test assertions must hold in an `if`-free subshell (`t()` handles that). Test expectations must come from independent sources, e.g. Catppuccin's published hex values, not our own data files.
- On the author's WSL machine, an untracked `dotfiles/` copy of v1 is kept (excluded via `.git/info/exclude`) until `run_before_00-migrate-v1` replaces the old Stow symlinks.

## Constraints

The repo will be public: no identities, secrets or machine-specific values in tracked files. Commit with the GitHub noreply email.

## Agent skills

### Issue tracker

GitHub Issues on neelanjan-mukherji/dev-environment, via the `gh` CLI. See `docs/agents/issue-tracker.md`.

### Triage labels

The five default triage labels: needs-triage, needs-info, ready-for-agent, ready-for-human, wontfix. See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: `GLOSSARY.md` and `docs/adr/` at the repo root, created only when needed. See `docs/agents/domain.md`.

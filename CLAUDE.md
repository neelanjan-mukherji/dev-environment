# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

Personal workstation provisioning + dotfiles. Currently targets **Ubuntu 24.04 on WSL 2 only** (`install.sh` hard-fails on non-Linux / non-Ubuntu / root). There is no build step and no test suite.

## Commands

```bash
./install.sh --dry-run            # print every privileged/install step without running it
./install.sh [--no-docker] [--no-kubernetes] [--no-java] [--no-nvim]
bash scripts/validate.sh          # `bash -n` syntax check of every *.sh in the repo (the only "test")
bash bootstrap/stow-dotfiles.sh   # re-link dotfiles only (backs up conflicting real files first)
```

Validate a single script: `bash -n bootstrap/<file>.sh`. Zsh files are not covered by `validate.sh`; check them with `zsh -n <file>`.

## Architecture

**Two layers, deliberately separate:**

1. `install.sh` → `bootstrap/*.sh` (system provisioning). `install.sh` parses flags, exports `REPO_DIR`, `DRY_RUN`, `INSTALL_*`, then invokes each bootstrap script as `run bash <script>`. Dry-run is implemented only by the `run()` wrapper in `install.sh` — it skips the *whole* script, so bootstrap scripts themselves don't check `DRY_RUN`. Each bootstrap script is standalone (`set -Eeuo pipefail`) and should stay idempotent (guard with `command -v … || install`) because the documented update flow is `git pull --ff-only && ./install.sh`.
2. `dotfiles/<package>/…` (user config), symlinked into `$HOME` by GNU Stow. Each top-level dir under `dotfiles/` is a Stow package whose internal layout mirrors `$HOME` (e.g. `dotfiles/zsh/.config/zsh/aliases.zsh` → `~/.config/zsh/aliases.zsh`). The package list is hardcoded in `bootstrap/stow-dotfiles.sh` (`packages=(…)`); adding a new package directory requires adding it there and to the README's stow commands.

Because dotfiles are symlinks, **edits in `~/.config/...` are edits to this repo**. Notably `git config --global` writes into `dotfiles/git/.gitconfig` (that's how the `[user]` block got there).

### Zsh load order
`.zshenv` (PATH: `~/.local/bin`, `~/bin`, `~/.cargo/bin`) → `.zprofile` (SDKMAN for login shells) → `.zshrc`: bootstraps Zinit if missing → `plugins.zsh` (must load before `compinit`; fzf-tab requirement) → `compinit` → `completion.zsh` → `options history keybindings aliases functions` → zoxide/fzf/starship init → SDKMAN → `~/.zshrc.local` (untracked machine-local overrides; template in `config/environment.example`).

Aliases/integrations are guarded with `command -v` so the shell still works when a tool is missing — keep that pattern.

### Neovim
`init.lua` sets core options then loads `lua/config/{lazy,options,keymaps}.lua`; plugins are a single spec list in `lua/plugins/init.lua` (lazy.nvim, lockfile committed). Targets Neovim 0.11 native LSP API; Treesitter pinned to `master` branch for 0.11 compatibility. Clipboard is hardwired to `clip.exe`/`powershell.exe` (WSL-specific).

## Platform-specific spots (relevant to any cross-OS work)
- `install.sh` OS guards; all bootstrap scripts use `apt`/`dpkg --print-architecture`.
- Debian binary renames handled in `install-cli.sh` (`fdfind`→`fd`, `batcat`→`bat` symlinks in `~/.local/bin`).
- Neovim WSL clipboard in `init.lua`.
- Docker requires systemd in WSL; the installer intentionally does **not** edit `/etc/wsl.conf`.

## Known gaps
- `yazi` and `lazygit` have dotfile packages / shell functions but nothing in `bootstrap/` installs them.
- Kubernetes apt repo is pinned to `v1.34` in `install-kubernetes.sh`.

## Constraints
Repo is meant to be private but must never contain secrets — `.gitignore` blocks keys, `.env`, `.kube/`, `.ssh/`, `*.local`. Machine-specific settings go in `~/.zshrc.local`, not tracked files.

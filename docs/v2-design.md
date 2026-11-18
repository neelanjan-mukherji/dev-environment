# dev-environment v2 — design

Status: agreed 2026-11-17. Branch `v2`; `main` holds `v1.0.0` (Ubuntu/WSL-only, Stow + apt).

## Goal
One repo that is the go-to setup for every OS and machine, installed and maintained through a guided, hands-on CLI (`dev`), and fully scriptable for unattended use.

## Platforms
| Tier | Targets |
|---|---|
| Full | Ubuntu/Debian/WSL, Fedora, Arch, macOS — x86_64 + arm64 |
| `minimal` only | Alpine, headless servers/containers, no-sudo machines |
| Windows (host) | winget; WSL + systemd (`/etc/wsl.conf`, confirm before writing); Windows Terminal; Nerd Font; light PowerShell profile (starship + zoxide); shared nvim/git/starship/lazygit configs. Real work happens in WSL. |
| Out of scope | Mobile (Termux/iOS), NixOS |

## Repository layout
- `.chezmoiroot` = `home` — `home/` is the chezmoi source state (dotfiles, templates, run scripts).
- `cli/` — Go module for the `dev` binary.
- `home/.chezmoidata/packages.yaml` — base system package names per package manager (inside the source tree so templates can read it).
- `ci/` — container definitions and verification scripts.
- `install.sh` / `install.ps1` — fetch the `dev` release binary (checksum-verified) and start `dev setup`.
- Local clone lives at `~/dev-environment`; chezmoi `sourceDir` points there.
- Public at `github.com/neelanjan-mukherji/dev-environment` once milestone 1 is green. One-liner: `curl -fsSL https://raw.githubusercontent.com/neelanjan-mukherji/dev-environment/main/install.sh | sh`.

## Package strategy
1. **Base system** (zsh, git, curl, build tools, fonts) — native PM: apt / dnf / pacman / apk / brew / winget, driven by `packages.yaml`.
2. **CLI tools and languages** — mise for everything (replaces SDKMAN, nvm, cargo-built tools, ad-hoc GitHub-release downloads). Small CLI tools track `latest`; toolchains (Java, Node, kubectl, …) and nvim plugins (lazy-lock) are pinned.
3. **Docker** — nothing paid or licensed: Docker Engine on Linux, Colima on macOS, Docker Engine inside WSL on Windows.

## Profiles
Profiles plus per-component overrides; the selection is persisted per machine (chezmoi data / `machine.yaml`).
- **minimal**: zsh + Zinit plugins, starship, git, gh, neovim, tmux (+TPM), fzf, ripgrep, fd, bat, eza, zoxide, jq, jless, lazygit, yazi, btop, duf.
- **dev**: minimal + Docker, Node LTS, Python (uv), Go, Rust, kondo.
- **full**: dev + kubectl, helm, k9s, Java 8/17/21/25 (Corretto), Maven, Gradle, Spring Boot CLI.
- **Optional, off by default**: cloud CLIs (aws/gcloud/az); `desktop` (VS Code + extensions, reviewed macOS/Windows OS tweaks) — lowest priority.

## Shell, editor, look
- zsh with Zinit + starship; shared aliases/functions in a file both shells can read; lean `.bashrc` fallback with the same aliases and starship/zoxide/fzf integrations for hosts without zsh.
- `~/.zshrc.local` for machine-local overrides.
- Neovim: own config, cross-platform clipboard (`clip.exe` / `pbcopy` / `wl-copy` or `xclip`), Mason-managed LSP servers.
- JetBrainsMono Nerd Font; WezTerm (single Lua config) on all desktop OSes; Windows Terminal gets font + WSL default profile.
- Catppuccin Mocha across nvim, WezTerm, tmux, starship, bat, fzf, lazygit, yazi, btop, eza — one chezmoi variable to switch.

## Identity and secrets
- No identity or secrets in the repo. Git name/email go in untracked `~/.gitconfig.local` (included by the tracked `.gitconfig`); default email is the GitHub noreply address.
- `dev setup` offers: git identity → ed25519 SSH key generation (if absent) → `gh auth login` → `gh ssh-key add`. Every step skippable.

## The `dev` CLI (Go + Charm huh/bubbletea, released via GoReleaser per OS/arch)
- **Hands-on by default**: every step shows what it will do and the exact command; options run / skip / explain. Interactive moments (browser auth, codes) pause with explicit instructions. Risky actions (wsl.conf, `chsh`, overwriting dotfiles) show a diff or ask first.
- **Scriptable**: every prompt has a flag/config equivalent — `dev setup --profile minimal --yes`, `--config machine.yaml`. Underneath, answers flow to chezmoi as `DEVENV_*` env vars (milestone 1 `install.sh` uses the same mechanism).
- **No-sudo guardrails**: detect missing sudo, install what is possible user-locally via mise, report what was skipped (`~/.local/state/dev/skipped.log`).
- **Failure handling**: continue past a failed component, mark it, print a summary table, write a log to `~/.local/state/dev/` (`failed.log`), exit non-zero; `dev setup --resume` retries failures only.
- Commands: `setup`, `doctor` (same checks as CI), `update`, `add <tool>`, `profile`, `migrate-v1`. (As built: `add`/`remove` take a component, which changes the machine's selection, or any mise tool, which stays machine-local in `~/.config/mise/conf.d/local.toml`. `update` restores Neovim plugins to the repo's `lazy-lock.json` rather than bumping them, so the lock stays pinned.)
- `dev update`: `chezmoi update` + `mise upgrade` (unpinned) + Zinit/lazy.nvim/TPM plugin updates; OS package upgrades only with `--system`. Shells show a ≤ once-daily notice when the repo is behind; never auto-update.
- `dev migrate-v1` (auto-offered when v1 Stow links are detected; suggests the `full` profile): remove v1 links → `chezmoi diff` preview → backup → `chezmoi apply` → verify mise Java/Node → only then remove SDKMAN/nvm and v1 copies in `~/.local/bin` / `~/.cargo/bin` (the cargo toolchain itself stays: mise's Rust uses `~/.cargo`). Milestone 1's `install.sh` implements this order without Stow (it removes the links directly).

## Testing
- Containers: Ubuntu, Debian, Fedora, Arch, Alpine × amd64/arm64, profiles `minimal` and `full`.
- GitHub-hosted macOS and Windows runners.
- Checks: expected tools present, shellcheck, idempotency (second run is a no-op). Same suite runs locally via `dev doctor` / `make test`.

## Milestones (each tagged and merged to `main` when green)
1. ✅ **Foundation** (`v2.0.0-alpha.1`) — chezmoi restructure, `packages.yaml`, mise, v1 parity on Ubuntu/WSL, bash fallback, theme, container CI (Ubuntu/Debian). Migrate this machine.
2. ✅ **`dev` CLI** — guided setup, doctor, update, migrate-v1, `--yes`/`--config`, GoReleaser, `install.sh`.
3. **macOS** — Homebrew, Colima, WezTerm + font, macOS CI. (Built; the macOS CI job is its only real-machine check so far.)
4. **More Linux** — Fedora, Arch, Alpine minimal, no-sudo mode, arm64 CI. (As built: `dev setup --no-sudo` is remembered per machine as chezmoi data `nosudo`; on Alpine, Neovim and jless come from apk because mise only has glibc builds; Arch has no arm64 image, so it's tested on amd64 only; CI also runs an Intel Mac.)
5. **Windows** — `install.ps1`, winget, WSL + systemd, Windows Terminal, PowerShell profile, Windows CI. (As built: `dev` runs natively on the host with chezmoi; the shared configs are found through `XDG_CONFIG_HOME=~\.config`; `$PROFILE` gets one line loading `~\.config\powershell\profile.ps1`; Windows Terminal is themed through fragments, its `settings.json` only read for the WSL profile GUIDs; WSL is offered at the end of `dev setup` (`--wsl` under `--yes`), and systemd inside it is the Linux side's existing wsl.conf step. CI installs winget on the Windows Server runner first.)
6. **Desktop component** (low priority) — VS Code, OS tweaks.

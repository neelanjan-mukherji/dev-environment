# dev-environment

My workstation setup for every machine: one command installs the shell, editor,
CLI tools and languages, themed consistently, and re-running it keeps the
machine in sync.

- **Dotfiles:** [chezmoi](https://chezmoi.io) (source in `home/`)
- **Tools and languages:** [mise](https://mise.jdx.dev): no root needed, per-project versions
- **Base packages:** the OS package manager

> **Status: v2 milestone 1.** Supported today: Ubuntu / Debian, including WSL 2.
> macOS, Fedora/Arch/Alpine and Windows follow. See [docs/v2-design.md](docs/v2-design.md).
> The v1 Stow-based setup is tagged `v1.0.0`.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/neelanjan-mukherji/dev-environment/main/install.sh | sh
```

Or from a clone:

```bash
git clone https://github.com/neelanjan-mukherji/dev-environment.git ~/dev-environment
~/dev-environment/install.sh
```

The installer is hands-on:
1. It asks for a profile and your git identity, once per machine. The email
   defaults to your GitHub noreply address when one can be found.
2. On a v1 (Stow) machine it first removes the old symlinks, after asking, and
   suggests the `full` profile. Your `dotfiles/` copy is left untouched.
3. It shows which files will change and backs up the existing ones to `~/.dotfiles-backup/<timestamp>/`.
4. It prints each command before running it, and asks before risky steps (login
   shell, docker group, `/etc/wsl.conf`, removing SDKMAN, nvm or old v1 binaries
   once mise replaces them).

Steps it cannot do, for example without sudo, are skipped and listed at the end
(`~/.local/state/dev/skipped.log`) rather than failing the run. Failed steps are
listed too (`failed.log`); re-running retries them.

```bash
./install.sh --dry-run                     # show what would change; touches nothing
./install.sh --yes --profile minimal       # unattended (servers, containers, CI)
./install.sh --profile dev --add cloud --remove go
./install.sh --theme latte                 # catppuccin flavor for every tool
```

### Profiles

| Profile | Adds |
|---|---|
| `minimal` | zsh (Zinit) + bash fallback, starship, git, gh, Neovim, tmux, fzf, ripgrep, fd, bat, eza, zoxide, jq, jless, lazygit, yazi, btop, duf |
| `dev` | + Docker Engine, Node 26, Python 3.14 + uv, Go 1.27, Rust, kondo |
| `full` | + Corretto 8/17/21/25 (21 default), Maven, Gradle, Spring Boot CLI, kubectl 1.37, Helm, k9s |

Optional component, off in every profile: `cloud` (awscli, gcloud, azure-cli).
Toolchains are pinned; small CLI tools track `latest`.

### Per-machine settings

Answers live in `~/.config/chezmoi/chezmoi.toml` (`profile`, `name`, `email`,
`[data.components] add/remove`, `[data.theme] flavor`). Edit it, then run
`chezmoi init && chezmoi apply`.

Machine-local files that are never tracked: `~/.gitconfig.local` (identity,
generated), `~/.zshrc.local`, `~/.bashrc.local`, `~/.profile.local`.

## Day to day

```bash
chezmoi edit ~/.zshrc      # edit the source in ~/dev-environment/home, then apply
chezmoi diff               # what would change
chezmoi apply              # sync this machine with the repo
chezmoi update             # git pull + apply
mise upgrade               # newer versions of "latest" tools
mise use java@corretto-17  # per-project version (writes mise.toml)
```

Projects with `.nvmrc`, `.node-version`, `.java-version` or `.python-version`
files are honoured automatically.

## WSL notes

- **Docker needs systemd.** If it's off, the installer offers to add
  `[boot] systemd=true` to `/etc/wsl.conf`. After that, run `wsl.exe --shutdown`
  from Windows and reopen the terminal.
- **The docker group is root-equivalent.** Membership lets you run Docker without
  sudo, but it effectively grants root inside Linux.
- **Clipboard:** `clip.exe` is used automatically (`clipcopy` in tmux, `has("wsl")` in Neovim).

## Development

```bash
make check             # shellcheck, Go tests + render tests (no Docker; never touches $HOME)
make test FILTER=theme # one render test
make test-containers   # full install + verify in clean Ubuntu/Debian containers
make verify            # dev doctor + a second-install no-op check against this machine
```

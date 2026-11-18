# dev-environment

My workstation setup for every machine: one command installs the shell, editor,
CLI tools and languages, themed consistently, and re-running it keeps the
machine in sync.

- **Dotfiles:** [chezmoi](https://chezmoi.io) (source in `home/`)
- **Tools and languages:** [mise](https://mise.jdx.dev): no root needed, per-project versions
- **Base packages:** the OS package manager (apt, Homebrew)

> **Status: v2 milestone 4.** Supported today: Ubuntu / Debian (including WSL 2),
> Fedora, Arch and macOS, on x86_64 and arm64; Alpine with the `minimal` profile.
> Windows follows. See [docs/v2-design.md](docs/v2-design.md).
> The v1 Stow-based setup is tagged `v1.0.0`.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/neelanjan-mukherji/dev-environment/main/install.sh | sh
```

`install.sh` only fetches the `dev` CLI (the newest release for your OS/CPU,
checked against its `checksums.txt`) into `~/.local/bin` and runs `dev setup`.
From a clone, you can build `dev` yourself instead (needs Go):

```bash
git clone https://github.com/neelanjan-mukherji/dev-environment.git ~/dev-environment
~/dev-environment/install.sh --from-source
```

`dev setup` is hands-on:
1. It asks for a profile and your git identity in a short form, once per machine;
   later runs only confirm them. The email defaults to your GitHub noreply
   address when one can be found.
2. On a v1 (Stow) machine it first removes the old symlinks, after asking, and
   suggests the `full` profile. Your `dotfiles/` copy is left untouched.
3. It shows which files will change and backs up the existing ones to `~/.dotfiles-backup/<timestamp>/`.
4. It prints each command before running it, and asks before risky steps (login
   shell, docker group, `/etc/wsl.conf`, removing SDKMAN, nvm or old v1 binaries
   once mise replaces them).

5. Finally it offers an SSH key, `gh auth login` and adding the key to GitHub.
   Each is skippable.

A failed step doesn't stop the others. The summary table lists what failed
(`~/.local/state/dev/failed.log`, latest run only) and what was skipped, for
example without sudo (`skipped.log`); `dev setup --resume` retries just the
failed steps.

```bash
dev setup --dry-run                     # show what would change; touches nothing
dev setup --yes --profile minimal       # unattended (servers, containers, CI)
dev setup --profile dev --add cloud --remove go
dev setup --theme latte                 # catppuccin flavor for every tool
dev setup --config machine.yaml         # answers from a file; flags win
```

`machine.yaml` takes the flag names as keys:

```yaml
profile: dev
name: Jane Doe
email: 123+jane@users.noreply.github.com
add: [cloud]
yes: true
```

### Profiles

| Profile | Adds |
|---|---|
| `minimal` | zsh (Zinit) + bash fallback, starship, git, gh, Neovim, tmux, fzf, ripgrep, fd, bat, eza, zoxide, jq, jless, lazygit, yazi, btop, duf |
| `dev` | + Docker (Engine on Linux, Colima on macOS), Node 26, Python 3.14 + uv, Go 1.27, Rust, kondo |
| `full` | + Corretto 8/17/21/25 (21 default), Maven, Gradle, Spring Boot CLI, kubectl 1.37, Helm, k9s |

Optional component, off in every profile: `cloud` (awscli, gcloud, azure-cli).
Toolchains are pinned; small CLI tools track `latest`.

### Per-machine settings

```bash
dev profile                # show the profile and components
dev profile full           # switch (keeps your add/remove overrides)
dev add java cloud         # add components...
dev add terraform@1.9      # ...or any mise tool, for this machine only
dev remove go
```

Answers live in `~/.config/chezmoi/chezmoi.toml`; machine-only tools in
`~/.config/mise/conf.d/local.toml`.

Machine-local files that are never tracked: `~/.gitconfig.local` (identity,
generated), `~/.zshrc.local`, `~/.bashrc.local`, `~/.profile.local`.

## Day to day

```bash
dev update                 # pull, re-apply, upgrade tools within their pins, update plugins
dev update --system        # ...and OS packages
dev doctor                 # does this machine match its profile?
chezmoi edit ~/.zshrc      # edit the source in ~/dev-environment/home, then `chezmoi apply`
mise use java@corretto-17  # per-project version (writes mise.toml)
```

Nothing updates by itself. When the repo has new commits, new shells say so (at
most once a day; `DEVENV_NO_UPDATE_CHECK=1` turns that off).

Projects with `.nvmrc`, `.node-version`, `.java-version` or `.python-version`
files are honoured automatically.

## macOS notes

- **Homebrew** installs the base packages, [WezTerm](https://wezterm.org) and
  JetBrainsMono Nerd Font. If Homebrew is missing, setup offers its official
  installer (it asks for your password and installs the Command Line Tools).
- **Docker without Docker Desktop:** the docker CLI with [Colima](https://github.com/abiosoft/colima)
  as the engine. Setup offers to start it now and at login (`brew services start colima`).
- **WezTerm:** themed like everything else; left Option is Meta, right Option
  still types symbols.
- macOS's `path_helper` reorders PATH for login shells; `~/.zprofile` puts
  `~/.local/bin` and the mise shims back in front.

## Linux notes

- **Package managers:** apt, dnf, pacman or apk installs only the base packages
  (`home/.chezmoidata/packages.yaml`); mise installs the rest. On Arch,
  installing packages means `pacman -Syu`, which upgrades the whole system.
- **Docker** comes from Docker's own repos on Ubuntu/Debian and Fedora/RHEL, and
  from Arch's packages on Arch.
- **Alpine** supports the `minimal` profile only (and defaults to it): Node and
  Python have no musl builds. Neovim and jless come from apk. A fresh Alpine has
  no curl: `wget -qO- …/install.sh | sh` works too.
- **No sudo** (a shared server): `dev setup --no-sudo` never runs sudo, even
  where it would work. It's remembered for the machine (`--no-sudo=false` turns
  it off), and such a machine defaults to the `minimal` profile. Everything
  mise installs still lands in your home; base packages, Docker and the login
  shell change are skipped and listed in `skipped.log` (`dev doctor` reports
  them as skipped, not failed). Without sudo, git, curl and bash must already
  be installed, and zsh too if you want it as your shell.

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
make build             # cli/dist/dev
make test-containers   # full install + verify in clean Ubuntu/Debian containers
ci/run-containers.sh fedora:43 archlinux:latest alpine:3.22   # other distros
NOSUDO=1 PROFILES=minimal ci/run-containers.sh                 # a machine without sudo
make verify            # dev doctor + a second-install no-op check against this machine
```

Releases: pushing a `v*` tag runs GoReleaser (`.goreleaser.yaml`), which
publishes `dev_<os>_<arch>.tar.gz` and `checksums.txt` for `install.sh`.

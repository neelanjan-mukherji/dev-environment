# dev-environment

Reproducible Ubuntu 24.04 on WSL 2 workstation configuration. Configuration is
tracked in Git, GNU Stow manages symlinks, and `install.sh` provisions packages.

## Fresh setup

In elevated Windows PowerShell:
```powershell
wsl --install -d Ubuntu-24.04
```
Launch Ubuntu, create your Linux user, then:
```bash
sudo apt update && sudo apt install -y git curl ca-certificates
git clone git@github.com:YOUR_GITHUB_USERNAME/dev-environment.git ~/dev-environment
cd ~/dev-environment
./install.sh
```
Create `dev-environment` as a **private** GitHub repository first. Alternatively,
clone over HTTPS. Never commit credentials, private keys, `.env` files or kubeconfigs.

## Options
```bash
./install.sh                  # all components
./install.sh --dry-run
./install.sh --no-docker
./install.sh --no-kubernetes
./install.sh --no-java
./install.sh --no-nvim
```

After installation, restart the Ubuntu terminal and run `exec zsh`.
Authenticate with `gh auth login`. Verify with `docker version`, `docker compose
version`, `kubectl version --client`, `nvim --version`, `sdk version` and `mvn -version`.

## Components
- Zsh + Zinit, Starship, autosuggestions, fast syntax highlighting, fzf-tab
- GNU Stow, fzf, zoxide, jq, fd, eza, bat, duf, kondo, jless, ripgrep, tmux
- GitHub CLI
- Native Docker Engine, Buildx and Compose plugin
- kubectl, Helm and k9s (no cluster or credentials are provisioned)
- SDKMAN, Java 8/17/21/25 (Corretto identifiers resolved at install time), Maven,
  Spring Boot CLI
- Neovim + Lazy.nvim, Treesitter, LSP, completion, Telescope, Neo-tree, Git signs,
  formatting, diagnostics and statusline

## WSL systemd
Docker's system service requires systemd. If needed, add to `/etc/wsl.conf`:
```ini
[boot]
systemd=true
```
Then from Windows PowerShell run `wsl --shutdown`, relaunch Ubuntu, and:
```bash
sudo systemctl enable --now docker
```
The installer does not edit `/etc/wsl.conf`. Docker group membership is effectively
root-equivalent inside Linux; understand the security implication before using it.

## Stow
```bash
cd ~/dev-environment/dotfiles
stow --restow --target="$HOME" zsh git starship nvim tmux
```
The installer backs up conflicting regular files to `~/.dotfiles-backup/<timestamp>/`.
Remove managed links with `stow --delete --target="$HOME" zsh git starship nvim tmux`.

## Java
SDKMAN manages user-scoped Java and build tools. Inspect available builds with
`sdk list java`; switch using `sdk use java <identifier>` and persist using
`sdk default java <identifier>`. The installer resolves Amazon Corretto build
identifiers dynamically and warns if a requested major is unavailable.

## Updating
```bash
git pull --ff-only
./install.sh
```
Package upgrades remain an explicit maintenance task (`sudo apt update && sudo apt upgrade`).

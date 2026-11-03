#!/usr/bin/env bash
set -Eeuo pipefail
sudo apt-get install -y bat btop duf fd-find fzf jq ripgrep stow tmux tree xclip xsel zsh zsh-common cargo
mkdir -p "$HOME/.local/bin"
link_bin() { local src="$1" dst="$2"; command -v "$src" >/dev/null && [[ -e "$dst" ]] || { command -v "$src" >/dev/null && ln -s "$(command -v "$src")" "$dst"; }; }
link_bin fdfind "$HOME/.local/bin/fd"
link_bin batcat "$HOME/.local/bin/bat"

if ! command -v eza >/dev/null; then
 sudo install -d -m 0755 /etc/apt/keyrings
 curl -fsSL https://raw.githubusercontent.com/eza-community/eza/main/deb.asc | sudo gpg --dearmor --yes -o /etc/apt/keyrings/eza.gpg
 echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/eza.gpg] https://deb.gierens.de stable main" | sudo tee /etc/apt/sources.list.d/eza.list >/dev/null
 sudo apt-get update && sudo apt-get install -y eza
fi
command -v zoxide >/dev/null || curl -fsS https://raw.githubusercontent.com/ajeetdsouza/zoxide/main/install.sh | sh
command -v starship >/dev/null || curl -fsSL https://starship.rs/install.sh | sh -s -- --yes

if ! command -v jless >/dev/null; then
 arch="$(dpkg --print-architecture)"; case "$arch" in amd64) a=x86_64;; arm64) a=aarch64;; *) a="";; esac
 if [[ -n "$a" ]]; then
  tmp="$(mktemp -d)"; trap 'rm -rf "$tmp"' EXIT
  url="$(curl -fsSL https://api.github.com/repos/PaulJuliusMartinez/jless/releases/latest | jq -r '.assets[] | select(.name | test("linux.*'"$a"'.*tar.gz$")) | .browser_download_url' | head -n1)"
  if [[ -n "$url" && "$url" != null ]]; then
   curl -fsSL "$url" -o "$tmp/jless.tgz"; tar -xzf "$tmp/jless.tgz" -C "$tmp"
   bin="$(find "$tmp" -type f -name jless -perm -111 | head -n1)"
   [[ -z "$bin" ]] || install -m 0755 "$bin" "$HOME/.local/bin/jless"
  else echo "jless release asset not found; skipping."; fi
 fi
fi
if ! command -v kondo >/dev/null; then cargo install kondo; fi
grep -Fq 'export PATH="$HOME/.local/bin:$PATH"' "$HOME/.profile" 2>/dev/null || echo 'export PATH="$HOME/.local/bin:$PATH"' >> "$HOME/.profile"

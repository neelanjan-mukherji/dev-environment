#!/usr/bin/env bash
set -Eeuo pipefail
zsh_path="$(command -v zsh)"
grep -Fxq "$zsh_path" /etc/shells || echo "$zsh_path" | sudo tee -a /etc/shells >/dev/null
mkdir -p "$HOME/.local/share"

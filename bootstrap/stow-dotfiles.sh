#!/usr/bin/env bash
set -Eeuo pipefail
repo="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "$repo/dotfiles"
backup="$HOME/.dotfiles-backup/$(date +%Y%m%d-%H%M%S)"
mkdir -p "$backup"
for package in zsh git starship nvim tmux; do
 while IFS= read -r source; do
  rel="${source#"$package"/}"; target="$HOME/$rel"
  if [[ -e "$target" && ! -L "$target" ]]; then
   mkdir -p "$backup/$(dirname "$rel")"; mv "$target" "$backup/$rel"
   echo "Backed up $target to $backup/$rel"
  fi
 done < <(find "$package" \( -type f -o -type l \))
done
stow --restow --target="$HOME" zsh git starship nvim tmux

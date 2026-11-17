#!/usr/bin/env bash
set -Eeuo pipefail

repo="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "$repo/dotfiles"

backup="$HOME/.dotfiles-backup/$(date +%Y%m%d-%H%M%S)"
packages=(zsh git starship nvim tmux yazi lazygit)

for package in "${packages[@]}"; do
    while IFS= read -r -d '' source; do
        rel="${source#"$package"/}"
        target="$HOME/$rel"

        source_real="$(realpath -m "$source")"
        target_real="$(realpath -m "$target")"

        # Do not back up files already linked to the repository.
        if [[ "$source_real" == "$target_real" ]]; then
            continue
        fi

        # Back up existing regular files, not symlinks.
        if [[ -f "$target" && ! -L "$target" ]]; then
            mkdir -p "$backup/$(dirname "$rel")"
            mv -- "$target" "$backup/$rel"
            echo "Backed up $target to $backup/$rel"
        fi
    done < <(find "$package" \( -type f -o -type l \) -print0)
done

stow --restow --target="$HOME" "${packages[@]}"

echo "Dotfiles successfully stowed."

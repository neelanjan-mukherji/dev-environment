#!/usr/bin/env bash
# Make $HOME look like a v1 (GNU Stow) machine: file and directory symlinks into
# ~/dotfiles. ci/verify.sh checks the migration replaced the links and left the
# old tree untouched.
set -Eeuo pipefail
cd "$HOME"
mkdir -p dotfiles/zsh dotfiles/nvim/.config/nvim dotfiles/git .config
echo '# v1 zshrc' >dotfiles/zsh/.zshrc
echo '-- v1 init.lua' >dotfiles/nvim/.config/nvim/init.lua
printf '[user]\n\tname = V1 User\n' >dotfiles/git/.gitconfig
ln -s dotfiles/zsh/.zshrc .zshrc
ln -s dotfiles/git/.gitconfig .gitconfig
ln -s ../dotfiles/nvim/.config/nvim .config/nvim

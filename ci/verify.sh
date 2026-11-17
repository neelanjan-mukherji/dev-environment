#!/usr/bin/env bash
# Test code is passed as strings to child shells on purpose.
# shellcheck disable=SC2016
# Run inside a test container after install.sh: asserts the machine ended up the
# way the profile promises, and that a second install is a no-op.
# Usage: ci/verify.sh <minimal|dev|full>
set -uo pipefail

profile="${1:?usage: verify.sh <profile>}"
repo="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"
pass=0 fail=0
# The user's PATH (~/.local/bin for chezmoi and mise, then mise shims).
# shellcheck source=/dev/null
. "$HOME/.config/shell/path.sh"

check() { # check "description" command...
    local desc="$1"; shift
    if out="$("$@" 2>&1)"; then pass=$((pass + 1)); printf 'ok   %s\n' "$desc"
    else fail=$((fail + 1)); printf 'FAIL %s\n' "$desc"; printf '%s\n' "$out" | tail -40 | sed 's/^/    /'; fi
}
in_zsh()  { zsh -ic "$*" </dev/null; }
in_bash() { bash -ic "$*" </dev/null; }

echo "== profile: $profile"

# ---- every profile
for tool in rg fd bat eza fzf zoxide starship jq jless lazygit yazi duf gh nvim tmux btop; do
    check "zsh has $tool" in_zsh "command -v $tool"
done
check "neovim is pinned to 0.11"          in_zsh 'nvim --version | head -1 | grep -q "v0.11"'
check "zinit plugins load"                in_zsh 'whence -w zinit | grep -q function'
check "tmux plugin manager present"       test -x "$HOME/.tmux/plugins/tpm/tpm"
check "neovim plugins restored"           test -d "$HOME/.local/share/nvim/lazy/telescope.nvim"
check "bat theme compiled"                in_zsh 'bat --list-themes | grep -q "Catppuccin Mocha"'
check "bash fallback has tools + aliases" in_bash 'command -v starship && command -v rg && alias gs'
check "login shell is zsh"                bash -c 'getent passwd "$(id -un)" | cut -d: -f7 | grep -q zsh'
check "git identity from .gitconfig.local" bash -c '[ "$(git config user.email)" = ci@example.com ]'

# ---- dev and full
if [[ $profile == dev || $profile == full ]]; then
    for tool in node npm python uv go cargo kondo docker; do
        check "zsh has $tool" in_zsh "command -v $tool"
    done
    check "node is the pinned major"  in_zsh 'node --version | grep -q "^v26\."'
    check "docker compose plugin"     docker compose version
else
    check "minimal has no node"       bash -c '! zsh -ic "command -v node" </dev/null'
fi

# ---- full
if [[ $profile == full ]]; then
    for tool in java mvn gradle spring kubectl helm k9s; do
        check "zsh has $tool" in_zsh "command -v $tool"
    done
    check "default JDK is Corretto 21" in_zsh 'java -version 2>&1 | grep -q "Corretto-21"'
    for v in 8 17 25; do
        check "Corretto $v installed" in_zsh "mise where java@corretto-$v"
    done
fi

# ---- v1 migration (when ci/seed-v1.sh prepared the home)
if [[ -d "$HOME/dotfiles" ]]; then
    check "v1: profile defaulted to full"   bash -c 'chezmoi data --format json | grep -q "\"profile\": \"full\""'
    check "v1: .zshrc is a managed file"    bash -c '[ -f ~/.zshrc ] && [ ! -L ~/.zshrc ]'
    check "v1: nvim dir is no longer a link" bash -c '[ -d ~/.config/nvim ] && [ ! -L ~/.config/nvim ]'
    check "v1: old zshrc left untouched"     bash -c '[ "$(cat ~/dotfiles/zsh/.zshrc)" = "# v1 zshrc" ]'
    check "v1: old nvim tree left untouched" bash -c '[ "$(ls ~/dotfiles/nvim/.config/nvim)" = init.lua ]'
fi

# ---- idempotency: a second run changes nothing and re-runs no install step
# Scripts are excluded: run_after_ steps always show as pending (they re-check each apply).
check "dotfiles in sync after install" bash -c 'out="$(chezmoi status --exclude externals,scripts)" && { [ -z "$out" ] || { echo "$out"; chezmoi diff --exclude externals,scripts --no-pager | head -40; false; }; }'
second="$("$repo/install.sh" --yes 2>&1)"
check "second install succeeds"   test $? -eq 0
# Step headers are coloured; strip the escapes before matching.
# shellcheck disable=SC2001  # needs a regex
second_plain="$(sed 's/\x1b\[[0-9;]*m//g' <<<"$second")"
check "second install is a no-op" bash -c '! grep -E "==> (System packages|CLI tools|Docker Engine|Login shell|Theme caches|Retire)" <<<"$1"' _ "$second_plain"
check "no step failed" test ! -s "${XDG_STATE_HOME:-$HOME/.local/state}/dev/failed.log"

echo "---- $pass passed, $fail failed"
if [[ -s "${XDG_STATE_HOME:-$HOME/.local/state}/dev/skipped.log" ]]; then
    echo "skipped steps (expected in containers: docker daemon):"
    cut -f2- "${XDG_STATE_HOME:-$HOME/.local/state}/dev/skipped.log" | sed 's/^/    /'
fi
((fail == 0))

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

check() { # check "description" command...
    local desc="$1"; shift
    if out="$("$@" 2>&1)"; then pass=$((pass + 1)); printf 'ok   %s\n' "$desc"
    else fail=$((fail + 1)); printf 'FAIL %s\n' "$desc"; printf '%s\n' "$out" | tail -5 | sed 's/^/    /'; fi
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

# ---- idempotency: a second run changes nothing and re-runs no install step
check "dotfiles in sync after install" bash -c '[ -z "$(chezmoi status --exclude externals)" ]'
second="$("$repo/install.sh" --yes 2>&1)"
check "second install succeeds"   test $? -eq 0
check "second install is a no-op" bash -c '! grep -E "==> (System packages|CLI tools|Docker Engine|Login shell|Theme caches)" <<<"$1"' _ "$second"

echo "---- $pass passed, $fail failed"
if [[ -s "${XDG_STATE_HOME:-$HOME/.local/state}/dev/skipped.log" ]]; then
    echo "skipped steps (expected in containers: docker daemon):"
    cut -f2- "${XDG_STATE_HOME:-$HOME/.local/state}/dev/skipped.log" | sed 's/^/    /'
fi
((fail == 0))

#!/usr/bin/env bash
# Test code is passed as strings to child shells on purpose.
# shellcheck disable=SC2016
# Run after an install (in a test container, or `make verify` on a real machine):
# `dev doctor` asserts the machine matches its profile; this script adds what only
# makes sense right after an install: the v1 migration outcome and that a second
# install is a no-op.
# Usage: ci/verify.sh
set -uo pipefail

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

# ---- the machine matches its profile: the same checks as `dev doctor` anywhere
dev="$(command -v dev 2>/dev/null || echo "$repo/cli/dist/dev")"
# Its own report is the useful output here, so it is shown in full.
if "$dev" doctor; then pass=$((pass + 1)); else fail=$((fail + 1)); echo "FAIL dev doctor"; fi

# ---- v1 migration (when ci/seed-v1.sh prepared the home)
if [[ -d "$HOME/dotfiles" ]]; then
    check "v1: profile defaulted to full"   bash -c 'chezmoi data --format json | grep -q "\"profile\": \"full\""'
    check "v1: .zshrc is a managed file"    bash -c '[ -f ~/.zshrc ] && [ ! -L ~/.zshrc ]'
    check "v1: nvim dir is no longer a link" bash -c '[ -d ~/.config/nvim ] && [ ! -L ~/.config/nvim ]'
    check "v1: old zshrc left untouched"     bash -c '[ "$(cat ~/dotfiles/zsh/.zshrc)" = "# v1 zshrc" ]'
    check "v1: old nvim tree left untouched" bash -c '[ "$(ls ~/dotfiles/nvim/.config/nvim)" = init.lua ]'
fi

# ---- idempotency: a second run changes nothing and re-runs no install step
second="$("$dev" setup --yes --source "$repo" 2>&1)"
check "second install succeeds"   test $? -eq 0
# Step headers are coloured; strip the escapes before matching.
# shellcheck disable=SC2001  # needs a regex
second_plain="$(sed 's/\x1b\[[0-9;]*m//g' <<<"$second")"
check "second install is a no-op" bash -c '! grep -E "==> (System packages|CLI tools|Docker Engine|Login shell|Theme caches|Retire)" <<<"$1"' _ "$second_plain"
check "update check runs" bash -c '"$1" update --check && [ -s "${XDG_STATE_HOME:-$HOME/.local/state}/dev/update-check" ]' _ "$dev"
check "no step failed" test ! -s "${XDG_STATE_HOME:-$HOME/.local/state}/dev/failed.log"

echo "---- $pass passed, $fail failed"
if [[ -s "${XDG_STATE_HOME:-$HOME/.local/state}/dev/skipped.log" ]]; then
    echo "skipped steps (expected in containers: docker daemon):"
    cut -f2- "${XDG_STATE_HOME:-$HOME/.local/state}/dev/skipped.log" | sed 's/^/    /'
fi
((fail == 0))

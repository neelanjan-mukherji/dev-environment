# The dev-environment update notice, shared by zsh and bash (keep it valid in both).
# Shows at most what the last check found; checks again in the background once a
# day. Nothing updates by itself: `dev update` does that when you choose.
# Turn it off with DEVENV_NO_UPDATE_CHECK=1 (e.g. in ~/.zshrc.local's environment).
if [ -z "${DEVENV_NO_UPDATE_CHECK:-}" ] && command -v dev >/dev/null 2>&1; then
    _devenv_check="${XDG_STATE_HOME:-$HOME/.local/state}/dev/update-check"
    if [ -s "$_devenv_check" ]; then
        _devenv_behind="$(cat "$_devenv_check")"
        if [ "$_devenv_behind" -gt 0 ] 2>/dev/null; then
            printf 'dev-environment: %s new commit(s) upstream; run `dev update` when convenient.\n' "$_devenv_behind"
        fi
    fi
    if [ ! -f "$_devenv_check" ] || [ -n "$(find "$_devenv_check" -mmin +1440 2>/dev/null)" ]; then
        (dev update --check >/dev/null 2>&1 &)
    fi
    unset _devenv_check _devenv_behind
fi

# PATH for every shell: sourced by ~/.zshenv, ~/.zprofile, ~/.profile, ~/.bashrc
# and the install scripts. POSIX sh; keep it fast and side-effect free.
#
# Resulting order: ~/.local/bin, ~/bin, mise shims, ~/.cargo/bin, Homebrew (macOS),
# then the system. Shims make mise-managed tools visible to non-interactive
# programs; interactive shells put the real tool directories in front with
# `mise activate`. Directories already on PATH are moved, not duplicated: macOS's
# path_helper (/etc/zprofile, /etc/profile) puts the system first, and sourcing
# this again afterwards restores the order.
_devenv_front="$HOME/.local/bin:$HOME/bin:${XDG_DATA_HOME:-$HOME/.local/share}/mise/shims:$HOME/.cargo/bin"
for _dir in /opt/homebrew/bin /opt/homebrew/sbin; do
    [ -d "$_dir" ] && _devenv_front="$_devenv_front:$_dir"
done
case "$PATH" in
    "$_devenv_front" | "$_devenv_front":*) ;;
    *)
        _rest=":$PATH:"
        _list="$_devenv_front:"
        while [ -n "$_list" ]; do
            _dir="${_list%%:*}"
            _list="${_list#*:}"
            while :; do
                case "$_rest" in
                    *":$_dir:"*) _rest="${_rest%%":$_dir:"*}:${_rest#*":$_dir:"}" ;;
                    *) break ;;
                esac
            done
        done
        _rest="${_rest#:}"
        PATH="$_devenv_front${_rest:+:${_rest%:}}"
        ;;
esac
unset _devenv_front _dir _list _rest
export PATH

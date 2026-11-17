# PATH for every shell: sourced by ~/.zshenv, ~/.profile and the install scripts.
# POSIX sh; keep it fast and side-effect free.
#
# Resulting order: ~/.local/bin, ~/bin, mise shims, ~/.cargo/bin, then the system.
# Shims make mise-managed tools visible to non-interactive programs; interactive
# shells put the real tool directories in front with `mise activate`.
for _dir in "$HOME/.cargo/bin" "${XDG_DATA_HOME:-$HOME/.local/share}/mise/shims" "$HOME/bin" "$HOME/.local/bin"; do
    case ":$PATH:" in
        *":$_dir:"*) ;;
        *) PATH="$_dir:$PATH" ;;
    esac
done
unset _dir
export PATH

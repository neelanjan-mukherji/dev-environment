# Functions shared by zsh and bash. Keep this file valid in both shells.

# ==========================================
# Yazi: change the shell's working directory after exiting.
# ==========================================

y() {
    local tmp cwd
    tmp="$(mktemp -t "yazi-cwd.XXXXXX")" || return

    yazi "$@" --cwd-file="$tmp"

    if [ -f "$tmp" ]; then
        cwd="$(cat -- "$tmp")"
        [ -n "$cwd" ] && [ -d "$cwd" ] && builtin cd -- "$cwd"
    fi

    command rm -f -- "$tmp"
}

# ==========================================
# Zoxide: interactive directory navigation
# ==========================================

# Zinit defines a `zi` alias; ours wins.
unalias zi 2>/dev/null

zi() {
    local dir
    dir="$(zoxide query -l | fzf --height 40% --reverse)" || return
    [ -n "$dir" ] && builtin cd -- "$dir"
}

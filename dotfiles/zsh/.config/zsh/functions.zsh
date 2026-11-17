
# ==========================================
# Shell Functions
# ==========================================

# Yazi: change the shell's working directory after exiting.
function y() {
    local tmp
    tmp="$(mktemp -t "yazi-cwd.XXXXXX")" || return

    yazi "$@" --cwd-file="$tmp"

    if [[ -f "$tmp" ]]; then
        local cwd
        cwd="$(<"$tmp")"
        [[ -n "$cwd" && -d "$cwd" ]] && builtin cd -- "$cwd"
    fi

    rm -f -- "$tmp"
}

# ==========================================
# Zoxide: interactive directory navigation
# ==========================================

# Remove Zinit's zi alias.
unalias zi 2>/dev/null

function zi() {
    local dir

    dir="$(zoxide query -l | fzf --height 40% --reverse)" || return

    [[ -n "$dir" ]] && builtin cd -- "$dir"
}


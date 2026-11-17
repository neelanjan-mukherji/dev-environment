# ==========================================
# Interactive Shell Guard
# ==========================================

[[ -o interactive ]] || return

# ==========================================
# Zinit
# ==========================================

ZINIT_HOME="${XDG_DATA_HOME:-$HOME/.local/share}/zinit/zinit.git"

if [[ ! -d "$ZINIT_HOME" ]]; then
    mkdir -p "$(dirname "$ZINIT_HOME")"
    git clone https://github.com/zdharma-continuum/zinit.git "$ZINIT_HOME"
fi

source "$ZINIT_HOME/zinit.zsh"

# Load plugins before initializing completion.
source "$HOME/.config/zsh/plugins.zsh"

# ==========================================
# Completion
# ==========================================

autoload -Uz compinit
ZCOMPDUMP="${XDG_CACHE_HOME:-$HOME/.cache}/zsh/zcompdump"
mkdir -p "$(dirname "$ZCOMPDUMP")"
compinit -d "$ZCOMPDUMP"

source "$HOME/.config/zsh/completion.zsh"

# ==========================================
# Modular Configuration
# ==========================================

for config in options history keybindings aliases functions; do
    source "$HOME/.config/zsh/$config.zsh"
done

# ==========================================
# Shell Integrations
# ==========================================

command -v zoxide >/dev/null && eval "$(zoxide init zsh)"
command -v fzf >/dev/null && eval "$(fzf --zsh)"
command -v starship >/dev/null && eval "$(starship init zsh)"

# ==========================================
# SDKMAN
# ==========================================

export SDKMAN_DIR="$HOME/.sdkman"
[[ -s "$SDKMAN_DIR/bin/sdkman-init.sh" ]] && source "$SDKMAN_DIR/bin/sdkman-init.sh"

# Local overrides
[[ -f "$HOME/.zshrc.local" ]] && source "$HOME/.zshrc.local"

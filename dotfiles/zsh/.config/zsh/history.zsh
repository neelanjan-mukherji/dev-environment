# ==========================================
# History Configuration
# ==========================================

HISTFILE="${XDG_STATE_HOME:-$HOME/.local/state}/zsh/history"
HISTSIZE=50000
SAVEHIST=50000

mkdir -p "$(dirname "$HISTFILE")"

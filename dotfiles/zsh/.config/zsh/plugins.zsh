# ==========================================
# Zinit Plugins
# ==========================================

# Autosuggestions
ZSH_AUTOSUGGEST_HIGHLIGHT_STYLE='fg=8'
zinit light zsh-users/zsh-autosuggestions

# Syntax highlighting
zinit light zdharma-continuum/fast-syntax-highlighting

# Additional completion definitions
zinit light zsh-users/zsh-completions

# FZF completion integration
zinit light Aloxaf/fzf-tab

# ==========================================
# Node Version Manager
# ==========================================

export NVM_DIR="$HOME/.nvm"
[[ -s "$NVM_DIR/nvm.sh" ]] && source "$NVM_DIR/nvm.sh"

# ==========================================
# FZF Defaults
# ==========================================

export FZF_DEFAULT_OPTS="
    --height 60%
    --layout=reverse
    --border=rounded
    --info=inline
    --margin=1
"

if command -v fd >/dev/null 2>&1; then
    export FZF_DEFAULT_COMMAND='fd --type f --hidden --follow --exclude .git'
    export FZF_CTRL_T_COMMAND="$FZF_DEFAULT_COMMAND"
    export FZF_ALT_C_COMMAND='fd --type d --hidden --follow --exclude .git'
fi

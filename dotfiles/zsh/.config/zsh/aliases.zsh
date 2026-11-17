
# ==========================================
# Navigation and File Management
# ==========================================

if command -v eza >/dev/null 2>&1; then
    alias ls='eza --icons=auto'
    alias ll='eza -lah --icons=auto --git'
    alias la='eza -a --icons=auto'
    alias tree='eza --tree --icons=auto'
fi

command -v bat >/dev/null 2>&1 && alias cat='bat --paging=never'


# ==========================================
# Search and System Utilities
# ==========================================

command -v rg >/dev/null 2>&1 && alias grep='rg'
command -v duf >/dev/null 2>&1 && alias df='duf'
command -v fd >/dev/null 2>&1 && alias fdf='fd'


# ==========================================
# Git
# ==========================================

alias g='git'
alias gs='git status -sb'
alias gl='git log --oneline --graph --decorate'
alias gd='git diff'
alias gds='git diff --staged'
alias gc='git commit'
alias gp='git push'
alias gpl='git pull --ff-only'
alias gco='git checkout'
alias gb='git branch'


# ==========================================
# Docker and Kubernetes
# ==========================================

alias d='docker'
alias dc='docker compose'
alias dps='docker ps'

alias k='kubectl'
alias kgp='kubectl get pods'
alias kgs='kubectl get svc'


# ==========================================
# Global Pipe Aliases
# ==========================================

alias -g G='| grep'
alias -g J='| jq'
alias -g L='| less'
alias -g H='| head'
alias -g T='| tail'
alias -g W='| wc -l'


# ==========================================
# File Associations
# ==========================================

alias -s pdf='xdg-open'
alias -s png='xdg-open'
alias -s jpg='xdg-open'
alias -s jpeg='xdg-open'
alias -s html='xdg-open'

alias -s log='nvim'
alias -s json='nvim'
alias -s yaml='nvim'
alias -s yml='nvim'


# ==========================================
# FZF
# ==========================================

alias fzf-history='fc -rl 1 | fzf'
alias fzf-files='fzf --preview "bat --color=always --style=numbers {}"'
alias fzf-dirs='fd --type d --hidden --exclude .git | fzf'

# ==========================================
# Editor
# ==========================================

alias vim='nvim'
alias vi='nvim'
alias vimdiff='nvim -d'

export EDITOR='nvim'
export VISUAL='nvim'

command -v eza >/dev/null && { alias ls='eza --icons=auto'; alias ll='eza -lah --icons=auto --git'; alias la='eza -a --icons=auto'; alias tree='eza --tree --icons=auto'; }
command -v bat >/dev/null && alias cat='bat --paging=never'
command -v rg >/dev/null && alias grep='rg'
command -v duf >/dev/null && alias df='duf'
command -v fd >/dev/null && alias find='fd'
alias g='git' gs='git status -sb' gl='git log --oneline --graph --decorate' gd='git diff' gds='git diff --staged' gc='git commit' gp='git push' gpl='git pull --ff-only' gco='git checkout' gb='git branch'
alias d='docker' dc='docker compose' dps='docker ps' k='kubectl' kgp='kubectl get pods' kgs='kubectl get svc'
alias -g G='| grep' J='| jq' L='| less' H='| head' T='| tail' W='| wc -l'
alias -s pdf='xdg-open' png='xdg-open' jpg='xdg-open' jpeg='xdg-open' html='xdg-open' log='nvim' json='nvim' yaml='nvim' yml='nvim'

# zsh-only aliases. Shared aliases live in ~/.config/shell/aliases.sh.

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

if [[ "$OSTYPE" == darwin* ]]; then
    _opener=open
elif command -v wslview >/dev/null 2>&1; then
    _opener=wslview
else
    _opener=xdg-open
fi

for _ext in pdf png jpg jpeg html; do alias -s "$_ext=$_opener"; done
for _ext in log json yaml yml; do alias -s "$_ext=nvim"; done
unset _opener _ext

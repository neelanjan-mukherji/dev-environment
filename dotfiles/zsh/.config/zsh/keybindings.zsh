bindkey -e
bindkey '^[[A' up-line-or-search
bindkey '^[[B' down-line-or-search
bindkey '^[[H' beginning-of-line
bindkey '^[[F' end-of-line
bindkey '^[[3~' delete-char
_insert_sudo() { LBUFFER="sudo $LBUFFER"; }
_insert_pipe_grep() { LBUFFER+=" | grep "; }
zle -N insert-sudo _insert_sudo
zle -N insert-pipe-grep _insert_pipe_grep
bindkey '^[s' insert-sudo
bindkey '^[g' insert-pipe-grep

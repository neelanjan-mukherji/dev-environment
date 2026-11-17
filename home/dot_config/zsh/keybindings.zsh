# ==========================================
# Keybindings
# ==========================================

bindkey -e

# Treat punctuation as word boundaries for Ctrl+Arrow navigation.
WORDCHARS=''

# Navigation
bindkey '^[[A' up-line-or-search
bindkey '^[[B' down-line-or-search
bindkey '^[[H' beginning-of-line
bindkey '^[[F' end-of-line
bindkey '^[[1~' beginning-of-line
bindkey '^[[4~' end-of-line

# Ctrl + Arrow
bindkey '^[[1;5D' backward-word
bindkey '^[[1;5C' forward-word

# Deletion
bindkey '^[[3~' delete-char
bindkey '^H' backward-kill-word
bindkey '^[[3;5~' kill-word
bindkey '^[^?' backward-kill-word
bindkey '^W' backward-kill-word

# Line editing
bindkey '^U' backward-kill-line
bindkey '^K' kill-line
bindkey '^A' beginning-of-line
bindkey '^E' end-of-line
bindkey '^D' delete-char
bindkey '^Y' yank
bindkey '^Z' undo

# Expand aliases after pressing Space
bindkey ' ' magic-space

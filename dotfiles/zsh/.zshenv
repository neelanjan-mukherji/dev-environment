# Ensure PATH contains unique entries.
typeset -U path PATH

# User-level binaries.
path=(
    "$HOME/.local/bin"
    "$HOME/bin"
    "$HOME/.cargo/bin"
    $path
)

export PATH

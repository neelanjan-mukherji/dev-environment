#!/bin/sh
# dev-environment installer.
#
#   curl -fsSL https://raw.githubusercontent.com/neelanjan-mukherji/dev-environment/main/install.sh | sh
#   ./install.sh                                   # from a clone: guided
#   ./install.sh --yes --profile minimal           # unattended
#
# Installs chezmoi and mise into ~/.local/bin, then lets chezmoi render the
# dotfiles in home/ and run its install scripts. Re-running is safe.
set -eu

REPO_URL="${DEVENV_REPO_URL:-https://github.com/neelanjan-mukherji/dev-environment.git}"
REPO_REF="${DEVENV_REF:-main}"
DEVENV_DIR="${DEVENV_DIR:-$HOME/dev-environment}"
BIN_DIR="$HOME/.local/bin"
STATE_DIR="${XDG_STATE_HOME:-$HOME/.local/state}/dev"

usage() {
    cat <<'EOF'
Usage: install.sh [options]

  --profile <minimal|dev|full>   what to install (asked if omitted)
  --name <name>                  git author name for ~/.gitconfig.local
  --email <email>                git author email (GitHub noreply recommended)
  --add <a,b>                    extra components (docker, devtools, node, python,
  --remove <a,b>                   go, rust, kubernetes, java, cloud)
  --theme <flavor>               catppuccin flavor: latte, frappe, macchiato, mocha
  --yes                          no questions: accept defaults and confirmations
  --dry-run                      show what would change, change nothing
  -h, --help                     this help

Answers are remembered in ~/.config/chezmoi/chezmoi.toml; later runs only ask
about new things. Skipped steps are listed at the end and in
~/.local/state/dev/skipped.log.
EOF
}

say()  { printf '\033[1;34m==>\033[0m \033[1m%s\033[0m\n' "$*"; }
info() { printf '    %s\n' "$*"; }
warn() { printf '\033[1;33m    ! %s\033[0m\n' "$*" >&2; }
die()  { printf '\033[1;31merror:\033[0m %s\n' "$*" >&2; exit 1; }

# Run a command, showing its output only if it fails.
quietly() {
    _log="$(mktemp)"
    if "$@" >"$_log" 2>&1; then rm -f "$_log"; return 0; fi
    cat "$_log" >&2; rm -f "$_log"; return 1
}

need_arg() { [ $# -ge 2 ] && [ -n "$2" ] || die "$1 needs a value"; }

DRY_RUN=0
DEVENV_YES="${DEVENV_YES:-0}"
while [ $# -gt 0 ]; do
    case "$1" in
        --profile) need_arg "$@"; DEVENV_PROFILE="$2"; export DEVENV_PROFILE; shift ;;
        --name)    need_arg "$@"; DEVENV_NAME="$2"; export DEVENV_NAME; shift ;;
        --email)   need_arg "$@"; DEVENV_EMAIL="$2"; export DEVENV_EMAIL; shift ;;
        --add)     need_arg "$@"; DEVENV_ADD="$2"; export DEVENV_ADD; shift ;;
        --remove)  need_arg "$@"; DEVENV_REMOVE="$2"; export DEVENV_REMOVE; shift ;;
        --theme)   need_arg "$@"; DEVENV_THEME="$2"; export DEVENV_THEME; shift ;;
        --yes | -y) DEVENV_YES=1 ;;
        --dry-run) DRY_RUN=1 ;;
        -h | --help) usage; exit 0 ;;
        *) usage >&2; die "unknown option: $1" ;;
    esac
    shift
done
export DEVENV_YES

ask() { # ask "Question?" -> 0 for yes
    [ "$DEVENV_YES" = 1 ] && return 0
    (exec </dev/tty) 2>/dev/null || return 1
    printf '\033[1;36m    ? %s [y/N] \033[0m' "$1" >/dev/tty
    read -r answer </dev/tty || return 1
    case "$answer" in y | Y | yes | YES) return 0 ;; *) return 1 ;; esac
}

# ------------------------------------------------------------------ preflight

case "$(uname -s)" in
    Linux) ;;
    Darwin) die "macOS support arrives in milestone 3." ;;
    *) die "Unsupported OS: $(uname -s). On Windows, run this inside WSL." ;;
esac
[ "$(id -u)" -ne 0 ] || [ "${DEVENV_ALLOW_ROOT:-0}" = 1 ] ||
    die "Run as your normal user, not root (sudo is used only where needed)."

if [ "$DRY_RUN" = 1 ]; then
    # A dry run must not touch $HOME: tools and config go to a scratch dir.
    tmp="$(mktemp -d)"
    trap 'rm -rf "$tmp"' EXIT
    BIN_DIR="$tmp/bin"
fi
mkdir -p "$BIN_DIR"
PATH="$BIN_DIR:$PATH"
export PATH

if ! command -v git >/dev/null 2>&1 || ! command -v curl >/dev/null 2>&1; then
    say "Installing prerequisites (git, curl)"
    command -v apt-get >/dev/null 2>&1 || die "Install git and curl, then re-run."
    sudo apt-get update -qq
    sudo env DEBIAN_FRONTEND=noninteractive apt-get install -y -qq git curl ca-certificates
fi

# ------------------------------------------------------------------ source

script_dir=""
case "$0" in */*) script_dir="$(cd -- "$(dirname -- "$0")" && pwd -P)" ;; esac
if [ -n "$script_dir" ] && [ -f "$script_dir/home/.chezmoi.toml.tmpl" ]; then
    repo="$script_dir"
elif [ -d "$DEVENV_DIR/.git" ]; then
    repo="$DEVENV_DIR"
    say "Updating $repo"
    git -C "$repo" pull --ff-only --quiet || warn "Could not fast-forward $repo; using it as is."
else
    say "Cloning $REPO_URL into $DEVENV_DIR"
    git clone --quiet --branch "$REPO_REF" "$REPO_URL" "$DEVENV_DIR"
    repo="$DEVENV_DIR"
fi
info "Source: $repo"

# ------------------------------------------------------------------ tools

if ! command -v chezmoi >/dev/null 2>&1; then
    say "Installing chezmoi into $BIN_DIR"
    quietly sh -c "$(curl -fsSL https://get.chezmoi.io)" -- -b "$BIN_DIR"
fi
if ! command -v mise >/dev/null 2>&1; then
    say "Installing mise into $BIN_DIR"
    curl -fsSL https://mise.run -o "$BIN_DIR/.mise-install.sh"
    quietly env MISE_INSTALL_PATH="$BIN_DIR/mise" sh "$BIN_DIR/.mise-install.sh"
    rm -f "$BIN_DIR/.mise-install.sh"
fi
info "chezmoi $(chezmoi --version | awk '{print $3}' | tr -d ,), mise $(mise --version | awk '{print $1}')"

# ------------------------------------------------------------------ configure

# Flags without spaces, safe to word-split.
init_extra=""
[ "$DEVENV_YES" = 1 ] && init_extra="--promptDefaults --no-tty"

if [ "$DRY_RUN" = 1 ]; then
    say "Dry run: rendering with a scratch config; nothing in \$HOME changes"
    set -- --config "$tmp/chezmoi.toml" --persistent-state "$tmp/state.boltdb" --cache "$tmp/cache"
    # shellcheck disable=SC2086
    chezmoi init --source "$repo" $init_extra "$@"
    chezmoi diff "$@" --exclude scripts,externals --no-pager || true
    say "Install scripts that would run"
    chezmoi managed "$@" --include scripts | sed 's/^/    /'
    exit 0
fi

say "Machine configuration"
# shellcheck disable=SC2086
chezmoi init --source "$repo" $init_extra
chezmoi data --format json 2>/dev/null |
    awk -F'"' '/"profile"|"flavor"/ { printf "    %s: %s\n", $2, $4 }'

mkdir -p "$STATE_DIR"

# ------------------------------------------------------------------ review

say "Files that will change"
changes="$(chezmoi status --exclude scripts,externals || true)"
if [ -z "$changes" ]; then
    info "None - dotfiles are up to date."
else
    printf '%s\n' "$changes" | sed 's/^/    /'
    backup="$HOME/.dotfiles-backup/$(date +%Y%m%d-%H%M%S)"
    printf '%s\n' "$changes" | while IFS= read -r line; do
        rel="${line#???}"
        target="$HOME/$rel"
        if [ -f "$target" ]; then
            mkdir -p "$backup/$(dirname "$rel")"
            cp -pL "$target" "$backup/$rel"
        fi
    done
    [ -d "$backup" ] && info "Existing versions backed up to $backup"
    if ! ask "Apply these changes and run the install steps?"; then
        info "Stopped. Review with: chezmoi diff"
        exit 0
    fi
fi

# ------------------------------------------------------------------ apply

skipped_before=0
[ -f "$STATE_DIR/skipped.log" ] && skipped_before="$(wc -l <"$STATE_DIR/skipped.log")"

status=0
chezmoi apply --force --keep-going || status=$?

say "Summary"
skipped_now=0
[ -f "$STATE_DIR/skipped.log" ] && skipped_now="$(wc -l <"$STATE_DIR/skipped.log")"
if [ "$skipped_now" -gt "$skipped_before" ]; then
    warn "Skipped this run (details in $STATE_DIR/skipped.log):"
    tail -n "$((skipped_now - skipped_before))" "$STATE_DIR/skipped.log" | cut -f2- | sed 's/^/      - /' >&2
fi
if [ "$status" -ne 0 ]; then
    warn "Some steps failed (see above). Fix the cause and re-run; finished steps are not repeated."
    exit "$status"
fi
info "Done. Open a new terminal (or run 'exec zsh'), then 'gh auth login'."

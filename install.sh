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

# Show a root command, then run it (directly when already root).
as_root() {
    printf '\033[2m    $ %s\033[0m\n' "$*" >&2
    if [ "$(id -u)" -eq 0 ]; then "$@"; else sudo "$@"; fi
}

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
    # A dry run must not change anything: tools, config and state go to a
    # scratch dir, and nothing is installed with sudo or fetched into $HOME.
    tmp="$(mktemp -d)"
    trap 'rm -rf "$tmp"' EXIT
    BIN_DIR="$tmp/bin"
    export MISE_DATA_DIR="$tmp/mise/data" MISE_STATE_DIR="$tmp/mise/state" \
        MISE_CACHE_DIR="$tmp/mise/cache" MISE_CONFIG_DIR="$tmp/mise/config"
fi
mkdir -p "$BIN_DIR"
PATH="$BIN_DIR:$PATH"
export PATH

if ! command -v git >/dev/null 2>&1 || ! command -v curl >/dev/null 2>&1; then
    [ "$DRY_RUN" = 0 ] || die "A dry run needs git and curl; install them first."
    command -v apt-get >/dev/null 2>&1 || die "Install git and curl, then re-run."
    [ "$(id -u)" -eq 0 ] || command -v sudo >/dev/null 2>&1 ||
        die "git and curl are missing and sudo is not available; install them as root, then re-run."
    say "Installing prerequisites (git, curl)"
    ask "Install git, curl and ca-certificates with apt (needs sudo)?" ||
        die "git and curl are required."
    as_root apt-get update -qq
    as_root env DEBIAN_FRONTEND=noninteractive apt-get install -y -qq git curl ca-certificates
fi

# ------------------------------------------------------------------ source

script_dir=""
case "$0" in */*) script_dir="$(cd -- "$(dirname -- "$0")" && pwd -P)" ;; esac
if [ -n "$script_dir" ] && [ -f "$script_dir/home/.chezmoi.toml.tmpl" ]; then
    repo="$script_dir"
elif [ -d "$DEVENV_DIR/.git" ]; then
    repo="$DEVENV_DIR"
    if [ "$DRY_RUN" = 0 ]; then
        say "Updating $repo"
        git -C "$repo" pull --ff-only --quiet || warn "Could not fast-forward $repo; using it as is."
    fi
elif [ "$DRY_RUN" = 1 ]; then
    say "Cloning $REPO_URL into a scratch dir"
    git clone --quiet --depth 1 --branch "$REPO_REF" "$REPO_URL" "$tmp/repo"
    repo="$tmp/repo"
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

# chezmoi flags for unattended runs; no spaces inside, safe to word-split.
prompt_flags=""
[ "$DEVENV_YES" = 1 ] && prompt_flags="--promptDefaults --no-tty"

# Print the machine's answers; extra args go to chezmoi (e.g. a scratch config).
show_settings() {
    chezmoi execute-template "$@" '    profile:    {{ .profile }}
    components: {{ includeTemplate "components" . | default "none" }}
    git name:   {{ .name | default "(none)" }}
    git email:  {{ .email | default "(none)" }}
    theme:      catppuccin {{ .theme.flavor }}'
    echo
}

# v1 (GNU Stow) symlinks into a dotfiles/ tree. A v1 machine had everything that
# is now the "full" profile, so that becomes the suggested profile.
has_v1_links() {
    for f in .zshrc .gitconfig .tmux.conf .config/nvim .config/zsh; do
        [ -L "$HOME/$f" ] && readlink "$HOME/$f" | grep -q 'dotfiles/' && return 0
    done
    return 1
}
if [ -z "${DEVENV_PROFILE:-}" ] && has_v1_links; then
    DEVENV_DEFAULT_PROFILE="${DEVENV_DEFAULT_PROFILE:-full}"
    export DEVENV_DEFAULT_PROFILE
fi

if [ "$DRY_RUN" = 1 ]; then
    say "Dry run: rendering with a scratch config; nothing in \$HOME changes"
    set -- --config "$tmp/chezmoi.toml" --persistent-state "$tmp/state.boltdb" --cache "$tmp/cache"
    # shellcheck disable=SC2086
    chezmoi init --source "$repo" $prompt_flags "$@"
    show_settings "$@"
    chezmoi diff "$@" --exclude scripts,externals --no-pager || true
    say "Install scripts that would run"
    chezmoi managed "$@" --include scripts | sed 's/^/    /'
    exit 0
fi

say "Machine configuration"
# shellcheck disable=SC2086
chezmoi init --source "$repo" $prompt_flags
show_settings
# A wrong answer (say, a profile name typed into the name prompt) is cheap to fix
# now and annoying later, so a person gets to check before anything is applied.
while [ "$DEVENV_YES" != 1 ] && ! ask "Use these settings?"; do
    (exec </dev/tty) 2>/dev/null || die "Nobody to ask; pass --profile/--name/--email or --yes."
    chezmoi init --source "$repo" --prompt
    show_settings
done

mkdir -p "$STATE_DIR"

# ------------------------------------------------------------------ migrate v1

# Old Stow links must go before the preview and backup: otherwise both would
# follow links into the v1 dotfiles/ tree, and applying would write into it.
if has_v1_links || [ -n "$(find "$HOME/.config" -maxdepth 3 -type l -lname '*dotfiles/*' 2>/dev/null | head -1)" ]; then
    migrate="$STATE_DIR/migrate-v1.sh"
    chezmoi execute-template <"$repo/home/.chezmoiscripts/run_before_00-migrate-v1.sh.tmpl" >"$migrate"
    if ! bash "$migrate"; then
        rm -f "$migrate"
        die "The v1 symlinks are still in place, so nothing was applied."
    fi
    rm -f "$migrate"
fi

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

lines() { if [ -f "$1" ]; then wc -l <"$1"; else echo 0; fi; }
# new_entries <log> <lines before>: entries appended during this run.
new_entries() {
    _now="$(lines "$1")"
    [ "$_now" -gt "$2" ] && tail -n "$((_now - $2))" "$1" | cut -f2- | sed 's/^/      - /' >&2
    return 0
}
skipped_before="$(lines "$STATE_DIR/skipped.log")"
failed_before="$(lines "$STATE_DIR/failed.log")"

status=0
chezmoi apply --force --keep-going || status=$?

say "Summary"
if [ "$(lines "$STATE_DIR/skipped.log")" -gt "$skipped_before" ]; then
    warn "Skipped this run (details in $STATE_DIR/skipped.log):"
    new_entries "$STATE_DIR/skipped.log" "$skipped_before"
fi
if [ "$status" -ne 0 ]; then
    warn "Failed this run (log: $STATE_DIR/failed.log):"
    new_entries "$STATE_DIR/failed.log" "$failed_before"
    warn "Fix the cause and re-run; finished steps are not repeated."
    exit "$status"
fi
info "Done. Open a new terminal (or run 'exec zsh'), then 'gh auth login'."

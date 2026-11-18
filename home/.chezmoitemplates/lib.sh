# shellcheck shell=bash
# Shared helpers for install scripts. Included with {{ "{{" }} template "lib.sh" . {{ "}}" }}.
#
# Conventions (hands-on by default, scriptable with DEVENV_YES=1):
# - every system-changing command is echoed (to stderr) before it runs (`run`, `as_root`);
# - risky steps ask first (`confirm`), and --yes answers yes;
# - something that can't be done (no sudo, unsupported OS) is skipped and recorded
#   in ~/.local/state/dev/skipped.log instead of failing the whole setup;
# - a real failure exits non-zero and is recorded in ~/.local/state/dev/failed.log;
#   chezmoi retries the script on the next apply.

DEVENV_STATE="${XDG_STATE_HOME:-$HOME/.local/state}/dev"
mkdir -p "$DEVENV_STATE"
# Same PATH as the user's shells (installed tools may not be on PATH yet).
# shellcheck source=/dev/null
. {{ joinPath .chezmoi.sourceDir "private_dot_config/shell/path.sh" | quote }}

DEVENV_STEP="${DEVENV_STEP:-install script}"
step()  { DEVENV_STEP="$*"; printf '\n\033[1;35m==>\033[0m \033[1m%s\033[0m\n' "$*"; }
info()  { printf '    %s\n' "$*"; }
warn()  { printf '\033[1;33m    ! %s\033[0m\n' "$*" >&2; }
record_skip() {
    printf '%s\t%s\n' "$(date '+%F %T')" "$*" >>"$DEVENV_STATE/skipped.log"
    warn "Skipped: $*"
}
_record_failure() {
    local rc=$?
    [ "$rc" -eq 0 ] && return 0
    printf '%s\t%s (exit %s)\n' "$(date '+%F %T')" "$DEVENV_STEP" "$rc" >>"$DEVENV_STATE/failed.log"
    return "$rc"
}
trap _record_failure EXIT

assume_yes() { [ "${DEVENV_YES:-0}" = 1 ]; }
have_tty()   { (exec </dev/tty) 2>/dev/null; }

# confirm "Question?" — yes under --yes; no when there is nobody to ask.
confirm() {
    if assume_yes; then info "$1 -> yes (--yes)"; return 0; fi
    have_tty || return 1
    local answer
    printf '\033[1;36m    ? %s [y/N] \033[0m' "$1" >/dev/tty
    read -r answer </dev/tty || return 1
    case "$answer" in y | Y | yes | YES) return 0 ;; *) return 1 ;; esac
}

# Echo to stderr so the command stays visible when its output is redirected.
run() { printf '\033[2m    $ %s\033[0m\n' "$*" >&2; "$@"; }

as_root() {
    if [ "$(id -u)" = 0 ]; then run "$@"; else run sudo "$@"; fi
}

# True when root commands can run: already root, passwordless sudo, or a person
# at the terminal who can type the sudo password.
can_sudo() {
    [ "$(id -u)" = 0 ] && return 0
    command -v sudo >/dev/null 2>&1 || return 1
    sudo -n true 2>/dev/null && return 0
    # The redirect is for the person at the terminal, not for root.
    # shellcheck disable=SC2024
    have_tty && sudo -v </dev/tty
}

is_wsl() { grep -qi microsoft /proc/sys/kernel/osrelease 2>/dev/null; }

# login_shell [user]: the user's login shell (getent on Linux, Directory Services on macOS).
login_shell() {
    local user="${1:-$(id -un)}"
    if command -v getent >/dev/null 2>&1; then
        getent passwd "$user" | cut -d: -f7
    else
        dscl . -read "/Users/$user" UserShell 2>/dev/null | awk '{print $2}'
    fi
}

# find_brew: puts Homebrew on PATH if it is installed; false otherwise.
find_brew() {
    local b
    for b in "$(command -v brew 2>/dev/null)" /opt/homebrew/bin/brew /usr/local/bin/brew; do
        if [ -n "$b" ] && [ -x "$b" ]; then
            eval "$("$b" shellenv)"
            return 0
        fi
    done
    return 1
}

# brew_install <formula|--cask cask>...: installs what is missing, one call.
brew_install() {
    local kind=--formula missing=() have
    if [ "${1:-}" = --cask ]; then kind=--cask; shift; fi
    have="$(brew list "$kind" -1 2>/dev/null || true)"
    local pkg
    for pkg in "$@"; do
        grep -qxF "$pkg" <<<"$have" || missing+=("$pkg")
    done
    if [ "${#missing[@]}" -eq 0 ]; then
        info "Already installed ($kind): $*"
        return 0
    fi
    run brew install "$kind" "${missing[@]}"
}

# True on Debian, Ubuntu and their derivatives (apt-based).
is_debian_like() {
    # shellcheck source=/dev/null
    (. /etc/os-release && case " ${ID:-} ${ID_LIKE:-} " in *" debian "* | *" ubuntu "*) exit 0 ;; *) exit 1 ;; esac)
}

# wsl.conf on stdin -> the same file with [boot] systemd=true on stdout. An existing
# systemd key is replaced, never duplicated; other sections and keys are kept.
wsl_conf_with_systemd() {
    awk '
        /^[[:space:]]*\[/ {
            if (in_boot && !done) { print "systemd=true"; done = 1 }
            in_boot = ($0 ~ /^[[:space:]]*\[boot\][[:space:]]*$/)
            print
            if (in_boot && !done) { print "systemd=true"; done = 1 }
            next
        }
        in_boot && /^[[:space:]]*systemd[[:space:]]*=/ { next }
        { print }
        END { if (!done) { print "[boot]"; print "systemd=true" } }
    '
}

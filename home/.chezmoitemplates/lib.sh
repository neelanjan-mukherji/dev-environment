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

# No-sudo machines (dev setup --no-sudo) never run sudo; the env var overrides for one run.
DEVENV_NO_SUDO="${DEVENV_NO_SUDO:-{{ if dig "nosudo" false . }}1{{ else }}0{{ end }}}"
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

# True when root commands can run: already root, or (unless this is a no-sudo
# machine) passwordless sudo or a person at the terminal who can type the password.
can_sudo() {
    [ "$(id -u)" = 0 ] && return 0
    [ "$DEVENV_NO_SUDO" = 1 ] && return 1
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

# package_manager: apt, dnf, pacman or apk, whichever this Linux has; empty if none.
package_manager() {
    local pm
    for pm in apt-get dnf pacman apk; do
        if command -v "$pm" >/dev/null 2>&1; then echo "${pm%-get}"; return 0; fi
    done
}

# pkg_installed <pm> <package>: true if the package is installed.
pkg_installed() {
    case "$1" in
        apt) dpkg-query -W -f='${Status}' "$2" 2>/dev/null | grep -q 'install ok installed' ;;
        dnf) rpm -q "$2" >/dev/null 2>&1 ;;
        pacman) pacman -Q "$2" >/dev/null 2>&1 ;;
        apk) apk info -e "$2" >/dev/null 2>&1 ;;
        *) return 1 ;;
    esac
}

# pkg_install <pm> <package>...: installs them as root, non-interactively.
pkg_install() {
    local pm="$1"; shift
    case "$pm" in
        apt) as_root apt-get update -qq
             as_root env DEBIAN_FRONTEND=noninteractive apt-get install -y -qq "$@" ;;
        dnf) as_root dnf install -y -q "$@" ;;
        # Arch has no partial upgrades: installing means syncing the whole system.
        pacman) as_root pacman -Syu --needed --noconfirm "$@" ;;
        apk) as_root apk add -q "$@" ;;
    esac
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

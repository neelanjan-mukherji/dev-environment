#!/bin/sh
# dev-environment bootstrap: puts the `dev` binary in ~/.local/bin, then runs
# `dev setup`, which does the actual work (see `dev setup --help`).
#
#   curl -fsSL https://raw.githubusercontent.com/neelanjan-mukherji/dev-environment/main/install.sh | sh
#   ./install.sh                                   # from a clone: guided
#   ./install.sh --yes --profile minimal           # unattended
#   ./install.sh --from-source                     # build dev from this clone (needs Go)
#
# Every other option goes to `dev setup`. Where dev comes from:
#   DEVENV_DEV_BIN=<path>   use this binary (CI builds it on the host)
#   --from-source           go build ./cli from this clone
#   otherwise               the newest GitHub release for this OS/arch, checked
#                           against the release's checksums.txt
# DEVENV_DEV_VERSION pins a release tag; DEVENV_RELEASE_URL replaces the download
# base URL (tests use a file:// directory).
set -eu

REPO="neelanjan-mukherji/dev-environment"
BIN_DIR="$HOME/.local/bin"

say() { printf '\033[1;34m==>\033[0m \033[1m%s\033[0m\n' "$*"; }
die() { printf '\033[1;31merror:\033[0m %s\n' "$*" >&2; exit 1; }

from_source=0
for arg in "$@"; do
    case "$arg" in
        --from-source) from_source=1 ;;
        -h | --help)
            sed -n '2,17p' "$0" 2>/dev/null | sed 's/^# \{0,1\}//'
            echo "Then: dev setup --help"
            exit 0 ;;
    esac
done

script_dir=""
case "$0" in */*) script_dir="$(cd -- "$(dirname -- "$0")" && pwd -P)" ;; esac
clone=""
[ -n "$script_dir" ] && [ -f "$script_dir/home/.chezmoi.toml.tmpl" ] && clone="$script_dir"

case "$(uname -s)" in
    Linux) os=linux ;;
    Darwin) os=darwin ;;
    *) die "Unsupported OS: $(uname -s). On Windows, run this inside WSL." ;;
esac
case "$(uname -m)" in
    x86_64 | amd64) arch=amd64 ;;
    aarch64 | arm64) arch=arm64 ;;
    *) die "Unsupported CPU: $(uname -m)." ;;
esac

sha256() {
    if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | cut -d' ' -f1
    else shasum -a 256 "$1" | cut -d' ' -f1; fi
}

# download <version|latest>: fetch, verify and install the release binary.
download() {
    base="${DEVENV_RELEASE_URL:-}"
    if [ -z "$base" ]; then
        tag="${DEVENV_DEV_VERSION:-}"
        if [ -z "$tag" ]; then
            # The newest release, prereleases included (v2 is in alpha).
            tag="$(curl -fsSL "https://api.github.com/repos/$REPO/releases?per_page=1" |
                sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -1)" ||
                die "Could not look up the latest release."
            [ -n "$tag" ] || die "No release published yet; clone the repo and run ./install.sh --from-source."
        fi
        base="https://github.com/$REPO/releases/download/$tag"
    fi
    asset="dev_${os}_${arch}.tar.gz"
    tmp="$(mktemp -d)"
    trap 'rm -rf "$tmp"' EXIT
    say "Downloading dev ($asset)"
    curl -fsSL "$base/$asset" -o "$tmp/$asset" || die "Download failed: $base/$asset"
    curl -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt" || die "Download failed: $base/checksums.txt"
    want="$(awk -v f="$asset" '$2 == f || $2 == "*" f { print $1 }' "$tmp/checksums.txt")"
    [ -n "$want" ] || die "checksums.txt has no entry for $asset."
    [ "$(sha256 "$tmp/$asset")" = "$want" ] || die "Checksum mismatch for $asset; not installing it."
    tar -xzf "$tmp/$asset" -C "$tmp" dev
    mkdir -p "$BIN_DIR"
    install -m 0755 "$tmp/dev" "$BIN_DIR/dev"
    rm -rf "$tmp"
    trap - EXIT
}

if [ -n "${DEVENV_DEV_BIN:-}" ]; then
    dev="$DEVENV_DEV_BIN"
elif [ "$from_source" = 1 ]; then
    [ -n "$clone" ] || die "--from-source needs a clone: git clone https://github.com/$REPO && ./dev-environment/install.sh --from-source"
    command -v go >/dev/null 2>&1 || die "--from-source needs Go on PATH."
    say "Building dev from $clone/cli"
    mkdir -p "$BIN_DIR"
    (cd "$clone/cli" && CGO_ENABLED=0 go build -trimpath -o "$BIN_DIR/dev" .)
    dev="$BIN_DIR/dev"
else
    download
    dev="$BIN_DIR/dev"
fi

# Drop the bootstrap-only flag; everything else is dev setup's.
n=$#
while [ "$n" -gt 0 ]; do
    arg="$1"; shift; n=$((n - 1))
    [ "$arg" = --from-source ] || set -- "$@" "$arg"
done
[ -n "$clone" ] && set -- --source "$clone" "$@"

# Under `curl | sh` stdin is the script; give dev setup the terminal to ask on.
if (exec </dev/tty) 2>/dev/null; then
    exec "$dev" setup "$@" </dev/tty
fi
exec "$dev" setup "$@"

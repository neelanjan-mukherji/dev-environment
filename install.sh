#!/usr/bin/env bash
set -Eeuo pipefail
REPO_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd -P)"
export REPO_DIR
DRY_RUN=0; INSTALL_DOCKER=1; INSTALL_KUBERNETES=1; INSTALL_JAVA=1; INSTALL_NVIM=1
usage() { echo "Usage: ./install.sh [--dry-run] [--no-docker] [--no-kubernetes] [--no-java] [--no-nvim]"; }
while (($#)); do
 case "$1" in
  --dry-run) DRY_RUN=1;; --no-docker) INSTALL_DOCKER=0;;
  --no-kubernetes) INSTALL_KUBERNETES=0;; --no-java) INSTALL_JAVA=0;;
  --no-nvim) INSTALL_NVIM=0;; -h|--help) usage; exit 0;;
  *) echo "Unknown option: $1" >&2; usage >&2; exit 2;;
 esac; shift
done
export DRY_RUN INSTALL_DOCKER INSTALL_KUBERNETES INSTALL_JAVA INSTALL_NVIM
run() { if [[ "$DRY_RUN" == 1 ]]; then printf '[dry-run]'; printf ' %q' "$@"; echo; else "$@"; fi; }
trap 's=$?; echo "Failed at line ${LINENO} (exit $s)" >&2; exit "$s"' ERR
[[ "$(uname -s)" == Linux ]] || { echo "Run inside Ubuntu/WSL." >&2; exit 1; }
grep -qi ubuntu /etc/os-release || { echo "Ubuntu is required." >&2; exit 1; }
[[ "$EUID" -ne 0 ]] || { echo "Run as normal user, not root." >&2; exit 1; }

run sudo apt-get update
run sudo apt-get install -y apt-transport-https bash-completion build-essential ca-certificates curl file git gnupg gpg lsb-release make unzip zip software-properties-common xz-utils
run bash "$REPO_DIR/bootstrap/install-cli.sh"
run bash "$REPO_DIR/bootstrap/install-gh.sh"
[[ "$INSTALL_DOCKER" == 0 ]] || run bash "$REPO_DIR/bootstrap/install-docker.sh"
[[ "$INSTALL_KUBERNETES" == 0 ]] || run bash "$REPO_DIR/bootstrap/install-kubernetes.sh"
[[ "$INSTALL_NVIM" == 0 ]] || run bash "$REPO_DIR/bootstrap/install-nvim.sh"
[[ "$INSTALL_JAVA" == 0 ]] || run bash "$REPO_DIR/bootstrap/install-sdkman.sh"
run bash "$REPO_DIR/bootstrap/configure-shell.sh"
run bash "$REPO_DIR/bootstrap/stow-dotfiles.sh"

ZSH_PATH="$(command -v zsh || true)"
if [[ -n "$ZSH_PATH" ]]; then
 if [[ "$DRY_RUN" == 1 ]]; then echo "[dry-run] sudo chsh -s $ZSH_PATH $USER"
 else sudo chsh -s "$ZSH_PATH" "$USER"; fi
fi
if [[ "$INSTALL_DOCKER" == 1 ]]; then
 if [[ "$DRY_RUN" == 1 ]]; then echo "[dry-run] sudo systemctl enable --now docker; sudo usermod -aG docker $USER"
 else
  if command -v systemctl >/dev/null && systemctl is-system-running >/dev/null 2>&1; then sudo systemctl enable --now docker
  else echo "systemd is not active. Enable it in /etc/wsl.conf; see README.md."; fi
  sudo usermod -aG docker "$USER"
 fi
fi
echo "Complete. Restart the Ubuntu terminal, run 'exec zsh', then 'gh auth login'."

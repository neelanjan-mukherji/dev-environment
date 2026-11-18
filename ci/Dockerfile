# Clean distro image with a sudo-capable normal user, like a fresh machine.
# Only sudo is added (and shadow's useradd where the image lacks one); git, curl,
# zsh and on Alpine even bash are left for dev setup to install.
# NOSUDO=1 builds a no-sudo machine instead (apt bases only): no sudo at all, and
# the base packages (packages.yaml's apt list) preinstalled by the "admin", like
# a shared server.
ARG BASE=ubuntu:24.04
FROM ${BASE}
ARG NOSUDO=0
COPY home/.chezmoidata/packages.yaml /tmp/packages.yaml
RUN set -e; \
    if command -v apt-get >/dev/null; then \
        apt-get update -qq; \
        if [ "$NOSUDO" = 1 ]; then \
            pkgs="$(awk '/^  apt:/ {f=1; next} /^  [a-z_]+:/ {f=0} f && /^    - / {print $2}' /tmp/packages.yaml)"; \
        else pkgs="sudo ca-certificates"; fi; \
        DEBIAN_FRONTEND=noninteractive apt-get install -y -qq --no-install-recommends $pkgs; \
        rm -rf /var/lib/apt/lists/*; \
    elif [ "$NOSUDO" = 1 ]; then \
        echo "NOSUDO=1 images are built from apt bases only (ubuntu, debian)" >&2; exit 1; \
    elif command -v dnf >/dev/null; then \
        dnf install -y -q sudo shadow-utils; dnf clean all; \
    elif command -v pacman >/dev/null; then \
        pacman -Syu --noconfirm --needed sudo; pacman -Scc --noconfirm; \
    elif command -v apk >/dev/null; then \
        apk add --no-cache sudo shadow; \
    fi; \
    rm -f /tmp/packages.yaml; \
    useradd --create-home --shell /bin/sh dev; \
    if [ "$NOSUDO" != 1 ]; then mkdir -p /etc/sudoers.d; echo 'dev ALL=(ALL) NOPASSWD:ALL' > /etc/sudoers.d/dev; fi
USER dev
WORKDIR /home/dev
COPY --chown=dev:dev . /home/dev/dev-environment

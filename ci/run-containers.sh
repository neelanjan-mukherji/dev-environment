#!/usr/bin/env bash
# Install + verify in clean containers.
# Usage: ci/run-containers.sh [base-image...]    (default: ubuntu:24.04 debian:13)
#        PROFILES="minimal full" ci/run-containers.sh fedora:43 archlinux:latest alpine:3.22
#        NOSUDO=1 ci/run-containers.sh           (a no-sudo machine; minimal only)
# The "full" profile runs as a v1 migration: the home starts with Stow links
# (ci/seed-v1.sh) and no --profile is given, so "full" must be the default.
# Alpine supports minimal only, and runs with no --profile: minimal must be its default.
# Needs Go on the host to build dev. Set GITHUB_TOKEN to avoid GitHub API rate limits while mise downloads tools.
set -Eeuo pipefail
repo="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"
bases=("$@"); ((${#bases[@]})) || bases=(ubuntu:24.04 debian:13)
read -r -a profiles <<<"${PROFILES:-minimal full}"
nosudo="${NOSUDO:-0}"
suffix="" note=""
[[ $nosudo = 1 ]] && suffix=-nosudo note=" (no sudo)"

# The dev binary goes into the image with the repo copy (cli/dist/dev), like a
# release download would put it on a fresh machine. Same CPU as the host.
(cd "$repo/cli" && CGO_ENABLED=0 GOOS=linux go build -trimpath -o dist/dev .)

results=()
for base in "${bases[@]}"; do
    tag="devenv-test:${base//[:\/]/-}$suffix"
    docker build -q --build-arg "BASE=$base" --build-arg "NOSUDO=$nosudo" -t "$tag" -f "$repo/ci/Dockerfile" "$repo" >/dev/null
    for profile in "${profiles[@]}"; do
        label="$base / $profile$note"
        args="--profile $profile" seed=0 expect=()
        if [[ $profile == full ]]; then
            [[ $base == alpine* || $nosudo = 1 ]] && { results+=("skip $label: minimal only"); continue; }
            args="" seed=1; echo "(starting from a v1 Stow home)"
        elif [[ $base == alpine* ]]; then
            args="" expect=(-e EXPECT_PROFILE=minimal)
        fi
        if [[ $nosudo = 1 ]]; then args="$args --no-sudo" expect+=(-e EXPECT_NO_SUDO=1); fi
        echo "######## $label"
        # sh, not bash: a fresh Alpine has no bash until dev setup installs it.
        if docker run --rm -e GITHUB_TOKEN -e MISE_GITHUB_TOKEN="${GITHUB_TOKEN:-}" "${expect[@]}" \
            -e DEVENV_DEV_BIN=/home/dev/dev-environment/cli/dist/dev "$tag" sh -c "
            { [ $seed = 0 ] || ./dev-environment/ci/seed-v1.sh; } &&
            ./dev-environment/install.sh --yes $args --name 'CI User' --email ci@example.com &&
            ./dev-environment/ci/verify.sh"; then
            results+=("ok   $label")
        else
            results+=("FAIL $label")
        fi
    done
done
printf '%s\n' "${results[@]}"
! printf '%s\n' "${results[@]}" | grep -q '^FAIL'

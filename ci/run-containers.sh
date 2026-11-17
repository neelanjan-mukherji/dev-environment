#!/usr/bin/env bash
# Install + verify in clean containers.
# Usage: ci/run-containers.sh [base-image...]    (default: ubuntu:24.04 debian:13)
#        PROFILES="minimal full" ci/run-containers.sh
# Set GITHUB_TOKEN to avoid GitHub API rate limits while mise downloads tools.
set -Eeuo pipefail
repo="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"
bases=("$@"); ((${#bases[@]})) || bases=(ubuntu:24.04 debian:13)
read -r -a profiles <<<"${PROFILES:-minimal full}"

results=()
for base in "${bases[@]}"; do
    tag="devenv-test:${base//[:\/]/-}"
    docker build -q --build-arg "BASE=$base" -t "$tag" -f "$repo/ci/Dockerfile" "$repo" >/dev/null
    for profile in "${profiles[@]}"; do
        echo "######## $base / $profile"
        if docker run --rm -e GITHUB_TOKEN -e MISE_GITHUB_TOKEN="${GITHUB_TOKEN:-}" "$tag" bash -c "
            ./dev-environment/install.sh --yes --profile $profile --name 'CI User' --email ci@example.com &&
            ./dev-environment/ci/verify.sh $profile"; then
            results+=("ok   $base / $profile")
        else
            results+=("FAIL $base / $profile")
        fi
    done
done
printf '%s\n' "${results[@]}"
! printf '%s\n' "${results[@]}" | grep -q '^FAIL'

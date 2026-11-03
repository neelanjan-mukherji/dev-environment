#!/usr/bin/env bash
set -Eeuo pipefail
[[ -s "$HOME/.sdkman/bin/sdkman-init.sh" ]] || curl -fsSL https://get.sdkman.io | bash
source "$HOME/.sdkman/bin/sdkman-init.sh"
sdk install maven || true
sdk install springboot || true
catalogue="$(sdk list java 2>/dev/null || true)"
for major in 8 17 21 25; do
 candidate="$(printf '%s\n' "$catalogue" | grep -Eo '([0-9]+(\.[0-9]+){1,3})-amzn' | awk -F. -v m="$major" '$1==m' | sort -Vu | tail -n1)"
 if [[ -n "$candidate" ]]; then sdk install java "$candidate" || echo "Could not install Java $major; inspect sdk list java."
 else echo "No Corretto identifier found for Java $major; inspect sdk list java."; fi
done

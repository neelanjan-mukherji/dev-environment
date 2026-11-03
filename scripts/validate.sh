#!/usr/bin/env bash
set -Eeuo pipefail
root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"
while IFS= read -r -d '' f; do bash -n "$f"; echo "OK: ${f#"$root"/}"; done < <(find "$root" -type f -name '*.sh' -print0)

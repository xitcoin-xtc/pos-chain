#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
status=0

while IFS= read -r entry; do
  file="${entry%%:*}"
  value="${entry#*:}"
  value="${value%%[[:space:]]#*}"
  value="${value//\"/}"
  value="${value//\'/}"
  value="${value//[[:space:]]/}"

  case "$value" in
    ./*) continue ;;
    docker://*)
      if [[ ! "$value" =~ ^docker://[^@]+@sha256:[0-9a-f]{64}$ ]]; then
        printf 'Docker action not pinned by digest: %s: %s\n' "$file" "$value" >&2
        status=1
      fi
      continue
      ;;
  esac

  if [[ ! "$value" =~ ^[^/@]+/[^/@]+@[0-9a-f]{40}$ ]]; then
    printf 'Mutable or invalid GitHub Action reference: %s: %s\n' "$file" "$value" >&2
    status=1
  fi
done < <(
  grep -RHE '^[[:space:]-]*uses:[[:space:]]*' "$repo_root/.github/workflows" |
    sed -E 's/^([^:]+):[[:space:]-]*uses:[[:space:]]*/\1:/'
)

exit "$status"

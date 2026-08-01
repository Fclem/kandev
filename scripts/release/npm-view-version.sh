#!/usr/bin/env bash
# Resolve one npm package/version spec. A missing package, version, or dist-tag
# is a successful empty result; registry and network failures are errors.
set -euo pipefail

if [[ "$#" -ne 1 || -z "$1" ]]; then
  echo "Usage: $0 <package-spec>" >&2
  exit 2
fi

SPEC="$1"
ERROR_FILE="$(mktemp)"
trap 'rm -f "$ERROR_FILE"' EXIT

if VERSION="$(npm view "$SPEC" version --loglevel=error 2>"$ERROR_FILE")"; then
  printf '%s\n' "$VERSION"
  exit 0
fi

if grep -qiE 'E404|404 Not Found|No match found for version|is not in this registry' "$ERROR_FILE"; then
  exit 0
fi

echo "npm view failed for $SPEC" >&2
exit 1

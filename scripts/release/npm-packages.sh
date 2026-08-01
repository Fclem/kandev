#!/usr/bin/env bash
# Shared npm package inventory for stable and Nightly publishing. Any platform
# change must also update package-npm-runtime.sh, apps/cli/src/runtime.ts,
# apps/cli/bin/native-shim.js, apps/cli/package.json, publish-npm.test.mjs, and
# their contract tests.

RUNTIME_PACKAGES=(
  "@kdlbs/runtime-linux-x64"
  "@kdlbs/runtime-linux-arm64"
  "@kdlbs/runtime-darwin-x64"
  "@kdlbs/runtime-darwin-arm64"
  "@kdlbs/runtime-win32-x64"
)
NIGHTLY_PACKAGES=("kandev" "${RUNTIME_PACKAGES[@]}")

readonly -a RUNTIME_PACKAGES NIGHTLY_PACKAGES

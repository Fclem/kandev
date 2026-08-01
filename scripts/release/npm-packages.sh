#!/usr/bin/env bash
# Shared npm package inventory for stable and Nightly publishing.

RUNTIME_PACKAGES=(
  "@kdlbs/runtime-linux-x64"
  "@kdlbs/runtime-linux-arm64"
  "@kdlbs/runtime-darwin-x64"
  "@kdlbs/runtime-darwin-arm64"
  "@kdlbs/runtime-win32-x64"
)
NIGHTLY_PACKAGES=("kandev" "${RUNTIME_PACKAGES[@]}")

readonly -a RUNTIME_PACKAGES NIGHTLY_PACKAGES

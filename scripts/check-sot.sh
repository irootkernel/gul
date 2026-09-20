#!/usr/bin/env bash
# Read-only checks using Node, already part of the pinned Gul toolchain.
set -eu
script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/.." && pwd)
exec node "$script_dir/check-sot.mjs" "$repo_root"

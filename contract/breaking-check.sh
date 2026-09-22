#!/usr/bin/env bash
set -eu

contract_root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
. "$contract_root/../toolchain/versions.env"

if [ "$#" -ne 2 ]; then
  printf 'usage: %s <module-directory> <baseline-descriptor>\n' "$0" >&2
  exit 2
fi

module_directory=$1
baseline_descriptor=$2
temporary_directory=$(mktemp -d "${TMPDIR:-/tmp}/gul-breaking-check.XXXXXX")
temporary_baseline="$temporary_directory/baseline.binpb"
cleanup() { rm -rf "$temporary_directory"; }
trap cleanup EXIT HUP INT TERM

if [ ! -d "$module_directory" ] || [ ! -r "$baseline_descriptor" ]; then
  printf 'ERROR breaking-check inputs are not readable\n' >&2
  exit 2
fi

if ! command -v buf >/dev/null 2>&1; then
  printf 'ERROR breaking-check requires Buf >= %s from PATH\n' "$GUL_BUF_MIN_VERSION" >&2
  exit 2
fi
buf_version=$(buf --version)
if ! "$contract_root/../scripts/toolchain-check.sh" --version-at-least "$buf_version" "$GUL_BUF_MIN_VERSION"; then
  printf 'ERROR breaking-check requires Buf >= %s; found %s\n' "$GUL_BUF_MIN_VERSION" "$buf_version" >&2
  exit 2
fi

cp "$baseline_descriptor" "$temporary_baseline"
buf breaking "$module_directory" --against "$temporary_baseline"

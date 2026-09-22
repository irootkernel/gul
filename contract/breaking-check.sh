#!/usr/bin/env bash
set -eu

contract_root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
. "$contract_root/../toolchain/versions.env"

version_cmp() {
  awk -v left="$1" -v right="$3" -v op="$2" '
    function component(value, position, parts, count) {
      count = split(value, parts, /[.]/)
      return position <= count ? parts[position] + 0 : 0
    }
    BEGIN {
      cmp = 0
      for (i = 1; i <= 4; i++) {
        a = component(left, i); b = component(right, i)
        if (a < b) { cmp = -1; break }
        if (a > b) { cmp = 1; break }
      }
      if (op == "ge") exit !(cmp >= 0)
      if (op == "lt") exit !(cmp < 0)
      exit 2
    }
  '
}

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
  printf 'ERROR breaking-check requires Buf >= %s and < %s from PATH\n' "$GUL_BUF_MIN_VERSION" "$GUL_BUF_MAX_EXCLUSIVE_VERSION" >&2
  exit 2
fi
buf_version=$(buf --version)
if ! version_cmp "$buf_version" ge "$GUL_BUF_MIN_VERSION" || ! version_cmp "$buf_version" lt "$GUL_BUF_MAX_EXCLUSIVE_VERSION"; then
  printf 'ERROR breaking-check requires Buf >= %s and < %s; found %s\n' "$GUL_BUF_MIN_VERSION" "$GUL_BUF_MAX_EXCLUSIVE_VERSION" "$buf_version" >&2
  exit 2
fi

cp "$baseline_descriptor" "$temporary_baseline"
buf breaking "$module_directory" --against "$temporary_baseline"

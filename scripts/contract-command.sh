#!/usr/bin/env bash
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
mode=${1-}

case "$mode" in
  generate) delegate="$script_dir/../contract/generate.sh" ;;
  check) delegate="$script_dir/../contract/check.sh" ;;
  *) printf 'usage: %s <generate|check>\n' "$0" >&2; exit 2 ;;
esac

if [ ! -x "$delegate" ]; then
  printf 'ERROR contract %s delegate is missing or not executable: %s\n' "$mode" "${delegate#"$script_dir/../"}" >&2
  exit 2
fi

exec "$delegate"

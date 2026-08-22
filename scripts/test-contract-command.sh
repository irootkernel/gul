#!/usr/bin/env bash
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
command_path="$script_dir/contract-command.sh"
fixture_dir=$(mktemp -d "${TMPDIR:-/tmp}/gul-contract-command.XXXXXX")
cleanup() { rm -rf "$fixture_dir"; }
trap cleanup EXIT HUP INT TERM

for mode in generate check; do
  set +e
  "$command_path" "$mode" >"$fixture_dir/out" 2>"$fixture_dir/err"
  gul_contract_rc=$?
  set -e
  if [ "$gul_contract_rc" -ne 2 ]; then
    printf 'ERROR contract %s returned %s instead of fail-closed exit 2\n' "$mode" "$gul_contract_rc" >&2
    exit 1
  fi
  if ! grep -q "contract $mode is unavailable until E0-T7" "$fixture_dir/err"; then
    printf 'ERROR contract %s did not name the E0-T7 prerequisite\n' "$mode" >&2
    cat "$fixture_dir/err" >&2
    exit 1
  fi
done

printf 'contract command prerequisites passed\n'

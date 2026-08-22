#!/usr/bin/env bash
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
command_path="$script_dir/contract-command.sh"
fixture_dir=$(mktemp -d "${TMPDIR:-/tmp}/gul-contract-command.XXXXXX")
cleanup() { rm -rf "$fixture_dir"; }
trap cleanup EXIT HUP INT TERM

set +e
"$command_path" invalid >"$fixture_dir/out" 2>"$fixture_dir/err"
fixture_rc=$?
set -e
if [ "$fixture_rc" -ne 2 ] || ! grep -q '<generate|check>' "$fixture_dir/err"; then
  printf 'ERROR contract command usage branch did not fail closed\n' >&2
  exit 1
fi

mkdir -p "$fixture_dir/scripts"
cp "$command_path" "$fixture_dir/scripts/contract-command.sh"
for mode in generate check; do
  set +e
  "$fixture_dir/scripts/contract-command.sh" "$mode" >"$fixture_dir/out" 2>"$fixture_dir/err"
  fixture_rc=$?
  set -e
  if [ "$fixture_rc" -ne 2 ] || ! grep -q "contract $mode delegate is missing or not executable" "$fixture_dir/err"; then
    printf 'ERROR contract %s missing-delegate branch did not fail closed\n' "$mode" >&2
    exit 1
  fi
done

printf 'contract command facade fixtures passed\n'

#!/usr/bin/env bash
set -eu

contract_root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
fixture_dir=$(mktemp -d "${TMPDIR:-/tmp}/gul-contract-check.XXXXXX")
cleanup() { rm -rf "$fixture_dir"; }
trap cleanup EXIT HUP INT TERM

"$contract_root/generate.sh" "$fixture_dir/generated"
if ! diff -ru "$contract_root/generated" "$fixture_dir/generated"; then
  printf 'ERROR generated contract drift detected\n' >&2
  exit 1
fi

cd "$contract_root"
go test ./...
bun run typecheck
bun scripts/validate-contract.mjs
printf 'contract drift and fixtures passed\n'

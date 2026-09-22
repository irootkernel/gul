#!/usr/bin/env bash
set -eu

contract_root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
fixture_directory=$(mktemp -d "${TMPDIR:-/tmp}/gul-breaking-fixture.XXXXXX")
cleanup() { rm -rf "$fixture_directory"; }
trap cleanup EXIT HUP INT TERM

"$contract_root/breaking-check.sh" \
  "$contract_root/upstream" \
  "$contract_root/upstream/baselines/dolgorae-public-v1-pre-task-053.descriptor.pb"

cp -R "$contract_root/upstream" "$fixture_directory/upstream"
perl -0pi -e 's/  rpc GetCapabilities\(GetCapabilitiesRequest\) returns \(GetCapabilitiesResponse\);\n//' \
  "$fixture_directory/upstream/dolgorae/public/v1/dolgorae.proto"
if "$contract_root/breaking-check.sh" \
  "$fixture_directory/upstream" \
  "$contract_root/upstream/baselines/dolgorae-public-v1-pre-task-053.descriptor.pb" \
  >"$fixture_directory/out" 2>"$fixture_directory/err"; then
  printf 'ERROR breaking descriptor fixture unexpectedly passed\n' >&2
  exit 1
fi

printf 'descriptor breaking-check fixtures passed\n'

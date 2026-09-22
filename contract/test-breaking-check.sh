#!/usr/bin/env bash
set -eu

contract_root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
# shellcheck disable=SC1091
. "$contract_root/../toolchain/versions.env"
fixture_directory=$(mktemp -d "${TMPDIR:-/tmp}/gul-breaking-fixture.XXXXXX")
system_buf=$(command -v buf)
cleanup() { rm -rf "$fixture_directory"; }
trap cleanup EXIT HUP INT TERM

"$contract_root/breaking-check.sh" \
  "$contract_root/upstream" \
  "$contract_root/upstream/baselines/dolgorae-public-v1-pre-task-053.descriptor.pb"

cp -R "$contract_root/upstream" "$fixture_directory/upstream"
perl -0pi -e 's/  rpc GetCapabilities\(GetCapabilitiesRequest\) returns \(GetCapabilitiesResponse\);\n//' \
  "$fixture_directory/upstream/dolgorae/public/v1/dolgorae.proto"
set +e
"$contract_root/breaking-check.sh" \
  "$fixture_directory/upstream" \
  "$contract_root/upstream/baselines/dolgorae-public-v1-pre-task-053.descriptor.pb" \
  >"$fixture_directory/out" 2>"$fixture_directory/err"
breaking_status=$?
set -e
if [ "$breaking_status" -ne 100 ] || ! grep -q 'Previously present RPC "GetCapabilities".*was deleted' "$fixture_directory/out" "$fixture_directory/err"; then
  printf 'ERROR breaking descriptor fixture did not report the expected RPC deletion (status %s)\n' "$breaking_status" >&2
  cat "$fixture_directory/out" "$fixture_directory/err" >&2
  exit 1
fi

mkdir "$fixture_directory/bin"
cat > "$fixture_directory/bin/buf" <<'EOF'
#!/bin/sh
printf '%s\n' 1.65.0
EOF
chmod +x "$fixture_directory/bin/buf"
set +e
PATH="$fixture_directory/bin:/usr/bin:/bin" "$contract_root/breaking-check.sh" \
  "$contract_root/upstream" \
  "$contract_root/upstream/baselines/dolgorae-public-v1-pre-task-053.descriptor.pb" \
  >"$fixture_directory/out" 2>"$fixture_directory/err"
minimum_status=$?
set -e
if [ "$minimum_status" -ne 2 ] || ! grep -q 'found 1.65.0' "$fixture_directory/err"; then
  printf 'ERROR breaking-check Buf minimum gate did not fail precisely\n' >&2
  exit 1
fi

cat > "$fixture_directory/bin/buf" <<'EOF'
#!/bin/sh
printf '%s\n' 1.66.1-rc.1
EOF
chmod +x "$fixture_directory/bin/buf"
set +e
PATH="$fixture_directory/bin:/usr/bin:/bin" "$contract_root/breaking-check.sh" \
  "$contract_root/upstream" \
  "$contract_root/upstream/baselines/dolgorae-public-v1-pre-task-053.descriptor.pb" \
  >"$fixture_directory/out" 2>"$fixture_directory/err"
prerelease_status=$?
set -e
if [ "$prerelease_status" -ne 2 ] || ! grep -q 'found 1.66.1-rc.1' "$fixture_directory/err"; then
  printf 'ERROR breaking-check accepted a below-minimum Buf prerelease\n' >&2
  exit 1
fi

cat > "$fixture_directory/bin/buf" <<EOF
#!/bin/sh
if [ "\${1-}" = "--version" ]; then
  printf '%s\n' 99.0.0
  exit 0
fi
exec "$system_buf" "\$@"
EOF
chmod +x "$fixture_directory/bin/buf"
PATH="$fixture_directory/bin:/usr/bin:/bin" "$contract_root/breaking-check.sh" \
  "$contract_root/upstream" \
  "$contract_root/upstream/baselines/dolgorae-public-v1-pre-task-053.descriptor.pb" \
  >"$fixture_directory/out" 2>"$fixture_directory/err"

mv "$fixture_directory/bin/buf" "$fixture_directory/bin/not-buf"
set +e
PATH="$fixture_directory/bin:/usr/bin:/bin" "$contract_root/breaking-check.sh" \
  "$contract_root/upstream" \
  "$contract_root/upstream/baselines/dolgorae-public-v1-pre-task-053.descriptor.pb" \
  >"$fixture_directory/out" 2>"$fixture_directory/err"
missing_status=$?
set -e
if [ "$missing_status" -ne 2 ] || ! grep -q 'requires Buf' "$fixture_directory/err"; then
  printf 'ERROR breaking-check missing-Buf gate did not fail precisely\n' >&2
  exit 1
fi

printf 'descriptor breaking-check fixtures passed\n'

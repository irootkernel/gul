#!/usr/bin/env bash
set -eu

contract_root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
version_manifest="$contract_root/../toolchain/versions.env"
output_root=${1:-"$contract_root/generated"}
wrapper_dir=$(mktemp -d "${TMPDIR:-/tmp}/gul-contract-tools.XXXXXX")
descriptor_path="$wrapper_dir/dolgorae-public-v1.descriptor.pb"
cleanup() { rm -rf "$wrapper_dir"; }
trap cleanup EXIT HUP INT TERM

cd "$contract_root"
. "$version_manifest"
export GOTOOLCHAIN=local

require_manifest_line() {
  local expected=$1
  if ! awk -v expected="$expected" '
    { line = $0; gsub(/^[[:space:]]+|[[:space:]]+$/, "", line); if (line == expected) count++ }
    END { exit count == 1 ? 0 : 1 }
  ' go.mod; then
    printf 'ERROR contract Go manifest must contain exactly one: %s\n' "$expected" >&2
    exit 2
  fi
}

if grep -Eq '^[[:space:]]*(replace|exclude)([[:space:]]|$)' go.mod; then
  printf 'ERROR contract Go manifest must not contain replace or exclude directives\n' >&2
  exit 2
fi
require_manifest_line "go ${GUL_GO_VERSION%.*}.0"
require_manifest_line "toolchain go$GUL_GO_VERSION"
require_manifest_line "connectrpc.com/connect v$GUL_CONNECT_GO_VERSION"
require_manifest_line "google.golang.org/protobuf v$GUL_PROTOBUF_GO_VERSION"
require_manifest_line 'connectrpc.com/connect/cmd/protoc-gen-connect-go'
require_manifest_line 'google.golang.org/protobuf/cmd/protoc-gen-go'

if ! go_line=$(go version 2>/dev/null); then
  printf 'ERROR contract generation requires Go exactly %s on darwin/%s; found unavailable\n' \
    "$GUL_GO_VERSION" "$GUL_TARGET_ARCH" >&2
  exit 2
fi
go_version=$(printf '%s\n' "$go_line" | sed -E 's/^go version go([^[:space:]]+).*/\1/')
go_platform=$(printf '%s\n' "$go_line" | awk '{print $4}')
if [ "$go_platform" != "darwin/$GUL_TARGET_ARCH" ]; then
  printf 'ERROR contract generation requires Go on darwin/%s; found %s\n' "$GUL_TARGET_ARCH" "${go_platform:-unknown}" >&2
  exit 2
fi
if [ "$go_version" != "$GUL_GO_VERSION" ]; then
  printf 'ERROR contract generation requires Go exactly %s; found %s\n' "$GUL_GO_VERSION" "$go_version" >&2
  exit 2
fi
if ! protoc_line=$(protoc --version 2>/dev/null); then
  printf 'ERROR contract generation requires protoc >= %s; found unavailable\n' "$GUL_PROTOC_MIN_VERSION" >&2
  exit 2
fi
protoc_version=$(printf '%s\n' "$protoc_line" | sed -E 's/^[^0-9]*//')
if ! "$contract_root/../scripts/toolchain-check.sh" --version-at-least "$protoc_version" "$GUL_PROTOC_MIN_VERSION"; then
  printf 'ERROR contract generation requires protoc >= %s; found %s\n' "$GUL_PROTOC_MIN_VERSION" "$protoc_version" >&2
  exit 2
fi
"$contract_root/../scripts/toolchain-check.sh" --bun-only >/dev/null
if ! buf_version=$(buf --version 2>/dev/null); then
  printf 'ERROR contract generation requires Buf >= %s; found unavailable\n' "$GUL_BUF_MIN_VERSION" >&2
  exit 2
fi
if ! "$contract_root/../scripts/toolchain-check.sh" --version-at-least "$buf_version" "$GUL_BUF_MIN_VERSION"; then
  printf 'ERROR contract generation requires Buf >= %s; found %s\n' \
    "$GUL_BUF_MIN_VERSION" "$buf_version" >&2
  exit 2
fi
for executable in node_modules/.bin/protoc-gen-es; do
  if [ ! -x "$executable" ]; then
    printf 'ERROR missing pinned contract dependency; run make test-prepare\n' >&2
    exit 2
  fi
done
if ! protoc_gen_go_version=$(go tool protoc-gen-go --version 2>/dev/null); then
  protoc_gen_go_version=unavailable
fi
if [ "$protoc_gen_go_version" != "protoc-gen-go v$GUL_PROTOC_GEN_GO_VERSION" ]; then
  printf 'ERROR contract generation requires protoc-gen-go %s; found %s\n' \
    "$GUL_PROTOC_GEN_GO_VERSION" "$protoc_gen_go_version" >&2
  exit 2
fi
if ! protoc_gen_connect_go_version=$(go tool protoc-gen-connect-go --version 2>/dev/null); then
  protoc_gen_connect_go_version=unavailable
fi
if [ "$protoc_gen_connect_go_version" != "$GUL_PROTOC_GEN_CONNECT_GO_VERSION" ]; then
  printf 'ERROR contract generation requires protoc-gen-connect-go %s; found %s\n' \
    "$GUL_PROTOC_GEN_CONNECT_GO_VERSION" "$protoc_gen_connect_go_version" >&2
  exit 2
fi
if ! protoc_gen_es_version=$(node_modules/.bin/protoc-gen-es --version 2>/dev/null); then
  protoc_gen_es_version=unavailable
fi
if [ "$protoc_gen_es_version" != "protoc-gen-es v$GUL_PROTOC_GEN_ES_VERSION" ]; then
  printf 'ERROR contract generation requires protoc-gen-es %s; found %s\n' \
    "$GUL_PROTOC_GEN_ES_VERSION" "$protoc_gen_es_version" >&2
  exit 2
fi
bun scripts/build-contract.mjs "$output_root"
(cd upstream && buf lint dolgorae/public/v1/dolgorae.proto)

buf build upstream --as-file-descriptor-set -o "$descriptor_path"
if ! cmp -s "$descriptor_path" upstream/dolgorae-public-v1.descriptor.pb; then
  pinned_buf_version=$(bun -e 'const value = await Bun.file("upstream/dolgorae-public-v1.descriptor.json").json(); process.stdout.write(value.generator.buf)')
  reproduced_sha256=$(shasum -a 256 "$descriptor_path" | awk '{print $1}')
  pinned_sha256=$(shasum -a 256 upstream/dolgorae-public-v1.descriptor.pb | awk '{print $1}')
  printf 'ERROR reproduced descriptor differs from the pinned TASK-053 descriptor\n' >&2
  printf 'running Buf %s; pinned descriptor producer Buf %s\n' "$buf_version" "$pinned_buf_version" >&2
  printf 'reproduced SHA-256 %s; pinned SHA-256 %s\n' "$reproduced_sha256" "$pinned_sha256" >&2
  printf 'compare by rerunning: buf build upstream --as-file-descriptor-set -o <path>\n' >&2
  exit 1
fi
./breaking-check.sh upstream upstream/baselines/dolgorae-public-v1-pre-task-053.descriptor.pb

mkdir -p "$wrapper_dir/bin" "$output_root/go" "$output_root/ts"
printf '#!/bin/sh\nexec go tool protoc-gen-go "$@"\n' > "$wrapper_dir/bin/protoc-gen-go"
printf '#!/bin/sh\nexec go tool protoc-gen-connect-go "$@"\n' > "$wrapper_dir/bin/protoc-gen-connect-go"
chmod +x "$wrapper_dir/bin/protoc-gen-go" "$wrapper_dir/bin/protoc-gen-connect-go"

NODE_NO_WARNINGS=1 PATH="$wrapper_dir/bin:$contract_root/node_modules/.bin:$PATH" protoc -I upstream \
  --descriptor_set_in=upstream/dolgorae-public-v1.descriptor.pb \
  --go_out="$output_root/go" \
  --go_opt=paths=source_relative \
  --go_opt=Mdolgorae/public/v1/dolgorae.proto=github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1 \
  --connect-go_out="$output_root/go" \
  --connect-go_opt=paths=source_relative \
  --connect-go_opt=Mdolgorae/public/v1/dolgorae.proto=github.com/rootkernel/gul/contract/generated/go/dolgorae/public/v1 \
  --es_out="$output_root/ts" \
  --es_opt=target=ts,import_extension=none \
  upstream/dolgorae/public/v1/dolgorae.proto

find "$output_root/ts" -type f -name '*.ts' -exec perl -0pi -e 's/\n+\z/\n/' {} +
gofmt -w "$output_root/go"
bun scripts/build-contract.mjs "$output_root" --lock-only
printf 'generated contract artifacts in %s\n' "$output_root"

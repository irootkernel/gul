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

version_cmp() {
  local left=$1
  local op=$2
  local right=$3
  awk -v left="$left" -v right="$right" -v op="$op" '
    function component(value, position, parts, count) {
      count = split(value, parts, /[.]/)
      return position <= count ? parts[position] + 0 : 0
    }
    BEGIN {
      cmp = 0
      for (i = 1; i <= 4; i++) {
        a = component(left, i)
        b = component(right, i)
        if (a < b) { cmp = -1; break }
        if (a > b) { cmp = 1; break }
      }
      if (op == "ge") exit !(cmp >= 0)
      if (op == "lt") exit !(cmp < 0)
      exit 2
    }
  '
}

if [ "$(go version)" != "go version go${GUL_GO_VERSION} darwin/arm64" ]; then
  printf 'ERROR contract generation requires Go %s on darwin/arm64\n' "$GUL_GO_VERSION" >&2
  exit 2
fi
if [ "$(protoc --version)" != "libprotoc $GUL_PROTOC_VERSION" ]; then
  printf 'ERROR contract generation requires protoc %s\n' "$GUL_PROTOC_VERSION" >&2
  exit 2
fi
"$contract_root/../scripts/toolchain-check.sh" --bun-only >/dev/null
buf_version=$(buf --version)
if ! version_cmp "$buf_version" ge "$GUL_BUF_MIN_VERSION" || ! version_cmp "$buf_version" lt "$GUL_BUF_MAX_EXCLUSIVE_VERSION"; then
  printf 'ERROR contract generation requires Buf >= %s and < %s; found %s\n' \
    "$GUL_BUF_MIN_VERSION" "$GUL_BUF_MAX_EXCLUSIVE_VERSION" "$buf_version" >&2
  exit 2
fi
for executable in node_modules/.bin/protoc-gen-es node_modules/.bin/protoc-gen-connect-es; do
  if [ ! -x "$executable" ]; then
    printf 'ERROR missing pinned contract dependency; run make test-prepare\n' >&2
    exit 2
  fi
done
if [ "$(go tool protoc-gen-go --version)" != "protoc-gen-go v$GUL_PROTOC_GEN_GO_VERSION" ]; then
  printf 'ERROR contract generation requires protoc-gen-go %s\n' "$GUL_PROTOC_GEN_GO_VERSION" >&2
  exit 2
fi
if [ "$(go tool protoc-gen-connect-go --version)" != "$GUL_PROTOC_GEN_CONNECT_GO_VERSION" ]; then
  printf 'ERROR contract generation requires protoc-gen-connect-go %s\n' "$GUL_PROTOC_GEN_CONNECT_GO_VERSION" >&2
  exit 2
fi
if [ "$(node_modules/.bin/protoc-gen-es --version)" != "protoc-gen-es v$GUL_PROTOC_GEN_ES_VERSION" ]; then
  printf 'ERROR contract generation requires protoc-gen-es %s\n' "$GUL_PROTOC_GEN_ES_VERSION" >&2
  exit 2
fi
if [ "$(node_modules/.bin/protoc-gen-connect-es --version)" != "protoc-gen-connect-es v$GUL_PROTOC_GEN_CONNECT_ES_VERSION" ]; then
  printf 'ERROR contract generation requires protoc-gen-connect-es %s\n' "$GUL_PROTOC_GEN_CONNECT_ES_VERSION" >&2
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
  --connect-es_out="$output_root/ts" \
  --connect-es_opt=target=ts,import_extension=none \
  upstream/dolgorae/public/v1/dolgorae.proto

find "$output_root/ts" -type f -name '*.ts' -exec perl -0pi -e 's/\n+\z/\n/' {} +
gofmt -w "$output_root/go"
bun scripts/build-contract.mjs "$output_root" --lock-only
printf 'generated contract artifacts in %s\n' "$output_root"

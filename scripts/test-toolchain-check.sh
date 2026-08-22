#!/usr/bin/env bash
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
checker="$script_dir/toolchain-check.sh"
manifest="$script_dir/../toolchain/versions.env"

# shellcheck disable=SC1090
. "$manifest"

fixture_dir=$(mktemp -d "${TMPDIR:-/tmp}/gul-toolchain.XXXXXX")
cleanup() { rm -rf "$fixture_dir"; }
trap cleanup EXIT HUP INT TERM

write_command() {
  local name=$1
  local output=$2
  local command_path="$fixture_dir/$name"
  printf '#!/bin/sh\nprintf "%%s\\n" "%s"\n' "$output" > "$command_path"
  chmod +x "$command_path"
}

cat > "$fixture_dir/uname" <<'EOF'
#!/bin/sh
case "${1-}" in
  -s) printf '%s\n' Darwin ;;
  -m) printf '%s\n' arm64 ;;
  *) exit 2 ;;
esac
EOF
chmod +x "$fixture_dir/uname"
write_command sw_vers 14.7.5
write_command go "go version go$GUL_GO_VERSION darwin/arm64"
write_command wails3 "v$GUL_WAILS_VERSION"
write_command node "v$GUL_NODE_VERSION"
write_command bun "$GUL_BUN_VERSION"
write_command buf "$GUL_BUF_VERSION"
write_command protoc "libprotoc $GUL_PROTOC_VERSION"
write_command protoc-gen-go "protoc-gen-go v$GUL_PROTOC_GEN_GO_VERSION"
write_command protoc-gen-connect-go "$GUL_PROTOC_GEN_CONNECT_GO_VERSION"
write_command protoc-gen-es "v$GUL_PROTOC_GEN_ES_VERSION"
write_command protoc-gen-connect-es "v$GUL_PROTOC_GEN_CONNECT_ES_VERSION"
write_command git "git version $GUL_GIT_MIN_VERSION"

PATH="$fixture_dir:/usr/bin:/bin" "$checker" >/dev/null

write_command wails3 v2.11.0
if PATH="$fixture_dir:/usr/bin:/bin" "$checker" >"$fixture_dir/out" 2>"$fixture_dir/err"; then
  printf 'ERROR mismatched Wails unexpectedly passed\n' >&2
  exit 1
fi
if ! grep -q "Wails: expected $GUL_WAILS_VERSION, found 2.11.0" "$fixture_dir/err"; then
  printf 'ERROR Wails mismatch was not reported precisely\n' >&2
  cat "$fixture_dir/err" >&2
  exit 1
fi

rm -f "$fixture_dir/wails3"
if PATH="$fixture_dir:/usr/bin:/bin" "$checker" >"$fixture_dir/out" 2>"$fixture_dir/err"; then
  printf 'ERROR missing Wails unexpectedly passed\n' >&2
  exit 1
fi
if ! grep -q "missing command 'wails3'" "$fixture_dir/err"; then
  printf 'ERROR missing Wails was not reported precisely\n' >&2
  cat "$fixture_dir/err" >&2
  exit 1
fi

printf 'toolchain checker fixtures passed\n'

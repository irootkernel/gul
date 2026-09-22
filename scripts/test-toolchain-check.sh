#!/usr/bin/env bash
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
checker="$script_dir/toolchain-check.sh"
manifest="$script_dir/../toolchain/versions.env"

# shellcheck disable=SC1090
. "$manifest"

if [ "$GUL_BUN_MIN_VERSION" != "0.3.14" ]; then
  printf 'ERROR Bun minimum must remain 0.3.14, found %s\n' "$GUL_BUN_MIN_VERSION" >&2
  exit 1
fi

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
write_command bun "$GUL_BUN_MIN_VERSION"
write_command buf 1.69.0
write_command protoc "libprotoc $GUL_PROTOC_VERSION"
write_command protoc-gen-go "protoc-gen-go v$GUL_PROTOC_GEN_GO_VERSION"
write_command protoc-gen-connect-go "$GUL_PROTOC_GEN_CONNECT_GO_VERSION"
write_command protoc-gen-es "v$GUL_PROTOC_GEN_ES_VERSION"
write_command git "git version $GUL_GIT_MIN_VERSION"

PATH="$fixture_dir:/usr/bin:/bin" "$checker" >/dev/null

write_command buf 1.65.0
if PATH="$fixture_dir:/usr/bin:/bin" "$checker" >"$fixture_dir/out" 2>"$fixture_dir/err"; then
  printf 'ERROR unsupported old Buf unexpectedly passed\n' >&2
  exit 1
fi
if ! grep -q "Buf: expected >= $GUL_BUF_MIN_VERSION and < $GUL_BUF_MAX_EXCLUSIVE_VERSION, found 1.65.0" "$fixture_dir/err"; then
  printf 'ERROR old Buf mismatch was not reported precisely\n' >&2
  cat "$fixture_dir/err" >&2
  exit 1
fi

write_command buf 2.0.0
if PATH="$fixture_dir:/usr/bin:/bin" "$checker" >"$fixture_dir/out" 2>"$fixture_dir/err"; then
  printf 'ERROR unsupported Buf major unexpectedly passed\n' >&2
  exit 1
fi
if ! grep -q "Buf: expected >= $GUL_BUF_MIN_VERSION and < $GUL_BUF_MAX_EXCLUSIVE_VERSION, found 2.0.0" "$fixture_dir/err"; then
  printf 'ERROR Buf upper bound was not reported precisely\n' >&2
  cat "$fixture_dir/err" >&2
  exit 1
fi

write_command buf 1.69.0

write_command bun 99.0.0
PATH="$fixture_dir:/usr/bin:/bin" "$checker" >/dev/null
PATH="$fixture_dir:/usr/bin:/bin" "$checker" --bun-only >/dev/null

write_command bun 1.3.13
PATH="$fixture_dir:/usr/bin:/bin" "$checker" >/dev/null
PATH="$fixture_dir:/usr/bin:/bin" "$checker" --bun-only >/dev/null

write_command bun 0.3.13
if PATH="$fixture_dir:/usr/bin:/bin" "$checker" >"$fixture_dir/out" 2>"$fixture_dir/err"; then
  printf 'ERROR unsupported old Bun unexpectedly passed\n' >&2
  exit 1
fi
if ! grep -q "Bun: expected >= $GUL_BUN_MIN_VERSION, found 0.3.13" "$fixture_dir/err"; then
  printf 'ERROR old Bun mismatch was not reported precisely\n' >&2
  cat "$fixture_dir/err" >&2
  exit 1
fi
if PATH="$fixture_dir:/usr/bin:/bin" "$checker" --bun-only >"$fixture_dir/out" 2>"$fixture_dir/err"; then
  printf 'ERROR unsupported old Bun unexpectedly passed --bun-only\n' >&2
  exit 1
fi
if ! grep -q "Bun: expected >= $GUL_BUN_MIN_VERSION, found 0.3.13" "$fixture_dir/err"; then
  printf 'ERROR old Bun --bun-only mismatch was not reported precisely\n' >&2
  cat "$fixture_dir/err" >&2
  exit 1
fi

write_command bun "$GUL_BUN_MIN_VERSION"
PATH="$fixture_dir:/usr/bin:/bin" "$checker" --bun-only >/dev/null

bun_only_dir="$fixture_dir/bun-only"
mkdir "$bun_only_dir"
cat > "$bun_only_dir/bun" <<EOF
#!/bin/sh
printf '%s\\n' '$GUL_BUN_MIN_VERSION'
EOF
chmod +x "$bun_only_dir/bun"
PATH="$bun_only_dir:/usr/bin:/bin" "$checker" --bun-only >/dev/null
if PATH="$bun_only_dir:/usr/bin:/bin" "$checker" >"$fixture_dir/out" 2>"$fixture_dir/err"; then
  printf 'ERROR --bun-only fixture did not isolate Bun from the full toolchain\n' >&2
  exit 1
fi

write_command bun 0.3.13
if PATH="$fixture_dir:$PATH" make -C "$script_dir/.." test-prepare >"$fixture_dir/out" 2>"$fixture_dir/err"; then
  printf 'ERROR test-prepare accepted an old system Bun\n' >&2
  exit 1
fi
if ! grep -q "Bun: expected >= $GUL_BUN_MIN_VERSION, found 0.3.13" "$fixture_dir/err"; then
  printf 'ERROR test-prepare did not stop at the Bun minimum gate\n' >&2
  exit 1
fi
if PATH="$fixture_dir:$PATH" "$script_dir/../contract/generate.sh" "$fixture_dir/generated" >"$fixture_dir/out" 2>"$fixture_dir/err"; then
  printf 'ERROR contract generation accepted an old system Bun\n' >&2
  exit 1
fi
if ! grep -q "Bun: expected >= $GUL_BUN_MIN_VERSION, found 0.3.13" "$fixture_dir/err"; then
  printf 'ERROR contract generation did not stop at the Bun minimum gate\n' >&2
  exit 1
fi
write_command bun "$GUL_BUN_MIN_VERSION"

rm "$fixture_dir/bun"
if PATH="$fixture_dir:/usr/bin:/bin" "$checker" --bun-only >"$fixture_dir/out" 2>"$fixture_dir/err"; then
  printf 'ERROR missing Bun unexpectedly passed\n' >&2
  exit 1
fi
if ! grep -q "missing command 'bun'" "$fixture_dir/err"; then
  printf 'ERROR missing Bun was not reported precisely\n' >&2
  cat "$fixture_dir/err" >&2
  exit 1
fi
write_command bun "$GUL_BUN_MIN_VERSION"

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

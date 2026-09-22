#!/usr/bin/env bash
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
checker="$script_dir/toolchain-check.sh"
manifest="$script_dir/../toolchain/versions.env"

# shellcheck disable=SC1090
. "$manifest"

if [ "$GUL_GO_VERSION" != "1.26.6" ]; then
  printf 'ERROR Go must remain exactly pinned to 1.26.6, found %s\n' "$GUL_GO_VERSION" >&2
  exit 1
fi

if [ "$GUL_BUN_MIN_VERSION" != "1.4.2" ]; then
  printf 'ERROR Bun minimum must remain 1.4.2, found %s\n' "$GUL_BUN_MIN_VERSION" >&2
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

write_unavailable_command() {
  local command_path="$fixture_dir/$1"
  printf '#!/bin/sh\nexit 127\n' > "$command_path"
  chmod +x "$command_path"
}

expect_version_at_least() {
  if ! "$checker" --version-at-least "$1" "$2"; then
    printf 'ERROR expected version %s to satisfy minimum %s\n' "$1" "$2" >&2
    exit 1
  fi
}

expect_version_below() {
  if "$checker" --version-at-least "$1" "$2"; then
    printf 'ERROR expected version %s to be below minimum %s\n' "$1" "$2" >&2
    exit 1
  fi
}

expect_version_at_least 3.0.0-beta.10 "$GUL_WAILS_MIN_VERSION"
expect_version_at_least 3.0.0 "$GUL_WAILS_MIN_VERSION"
expect_version_below 3.0.0-beta.7 "$GUL_WAILS_MIN_VERSION"
expect_version_below 1.26.6-rc.1 "$GUL_GO_VERSION"
expect_version_below 35.1-rc1 "$GUL_PROTOC_MIN_VERSION"
expect_version_below 35.1rc1 "$GUL_PROTOC_MIN_VERSION"
expect_version_below 1.66.1-rc.1 "$GUL_BUF_MIN_VERSION"

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
write_command wails3 "v$GUL_WAILS_MIN_VERSION"
write_command node "v$GUL_NODE_MIN_VERSION"
write_command bun "$GUL_BUN_MIN_VERSION"
write_command buf 1.69.0
write_command protoc "libprotoc $GUL_PROTOC_MIN_VERSION"
write_command git "git version $GUL_GIT_MIN_VERSION"

write_unavailable_command go
if PATH="$fixture_dir:/usr/bin:/bin" "$checker" >"$fixture_dir/out" 2>"$fixture_dir/err"; then
  printf 'ERROR missing Go unexpectedly passed the host check\n' >&2
  exit 1
fi
if ! grep -q "Go: missing command 'go'" "$fixture_dir/err"; then
  printf 'ERROR missing Go was not reported precisely by the host check\n' >&2
  cat "$fixture_dir/err" >&2
  exit 1
fi
write_command go "go version go$GUL_GO_VERSION darwin/arm64"

PATH="$fixture_dir:/usr/bin:/bin" "$checker" >/dev/null

write_command buf 1.65.0
if PATH="$fixture_dir:/usr/bin:/bin" "$checker" >"$fixture_dir/out" 2>"$fixture_dir/err"; then
  printf 'ERROR unsupported old Buf unexpectedly passed\n' >&2
  exit 1
fi
if ! grep -q "Buf: expected >= $GUL_BUF_MIN_VERSION, found 1.65.0" "$fixture_dir/err"; then
  printf 'ERROR old Buf mismatch was not reported precisely\n' >&2
  cat "$fixture_dir/err" >&2
  exit 1
fi

write_command buf 99.0.0
PATH="$fixture_dir:/usr/bin:/bin" "$checker" >/dev/null

write_command go "go version go$GUL_GO_VERSION darwin/arm64"
write_command wails3 'v3.0.0-beta.10'
write_command node 'v99.0.0'
write_command protoc 'libprotoc 99.0'
write_command git 'git version 99.0.0'
PATH="$fixture_dir:/usr/bin:/bin" "$checker" >/dev/null

write_command wails3 'v3.0.0'
PATH="$fixture_dir:/usr/bin:/bin" "$checker" >/dev/null

write_command go 'go version go1.27.0 darwin/arm64'
if PATH="$fixture_dir:/usr/bin:/bin" "$checker" >"$fixture_dir/out" 2>"$fixture_dir/err"; then
  printf 'ERROR newer non-pinned Go unexpectedly passed\n' >&2
  exit 1
fi
if ! grep -q "Go: expected exactly $GUL_GO_VERSION, found 1.27.0" "$fixture_dir/err"; then
  printf 'ERROR exact Go mismatch was not reported precisely\n' >&2
  cat "$fixture_dir/err" >&2
  exit 1
fi
if PATH="$fixture_dir:/usr/bin:/bin" "$checker" --go-only >"$fixture_dir/out" 2>"$fixture_dir/err"; then
  printf 'ERROR newer non-pinned Go unexpectedly passed --go-only\n' >&2
  exit 1
fi
if ! grep -q "Go: expected exactly $GUL_GO_VERSION, found 1.27.0" "$fixture_dir/err"; then
  printf 'ERROR exact Go --go-only mismatch was not reported precisely\n' >&2
  cat "$fixture_dir/err" >&2
  exit 1
fi

write_command go "go version go$GUL_GO_VERSION darwin/arm64"
PATH="$fixture_dir:/usr/bin:/bin" "$checker" --go-only >/dev/null
write_command wails3 "v$GUL_WAILS_MIN_VERSION"
write_command node "v$GUL_NODE_MIN_VERSION"
write_command buf 1.69.0
write_command protoc "libprotoc $GUL_PROTOC_MIN_VERSION"
write_command git "git version $GUL_GIT_MIN_VERSION"

write_command git 'git version 2.38.9'
if PATH="$fixture_dir:/usr/bin:/bin" "$checker" >"$fixture_dir/out" 2>"$fixture_dir/err"; then
  printf 'ERROR old Git unexpectedly passed\n' >&2
  exit 1
fi
if ! grep -q "Git: expected >= $GUL_GIT_MIN_VERSION, found 2.38.9" "$fixture_dir/err"; then
  printf 'ERROR old Git mismatch was not reported precisely\n' >&2
  cat "$fixture_dir/err" >&2
  exit 1
fi
write_unavailable_command git
if PATH="$fixture_dir:/usr/bin:/bin" "$checker" >"$fixture_dir/out" 2>"$fixture_dir/err"; then
  printf 'ERROR missing Git unexpectedly passed\n' >&2
  exit 1
fi
if ! grep -q "missing command 'git'" "$fixture_dir/err"; then
  printf 'ERROR missing Git was not reported precisely\n' >&2
  cat "$fixture_dir/err" >&2
  exit 1
fi
write_command git "git version $GUL_GIT_MIN_VERSION"

write_command bun 99.0.0
PATH="$fixture_dir:/usr/bin:/bin" "$checker" >/dev/null
PATH="$fixture_dir:/usr/bin:/bin" "$checker" --bun-only >/dev/null

write_command bun 1.4.3
PATH="$fixture_dir:/usr/bin:/bin" "$checker" >/dev/null
PATH="$fixture_dir:/usr/bin:/bin" "$checker" --bun-only >/dev/null

write_command bun 1.4.1
if PATH="$fixture_dir:/usr/bin:/bin" "$checker" >"$fixture_dir/out" 2>"$fixture_dir/err"; then
  printf 'ERROR unsupported old Bun unexpectedly passed\n' >&2
  exit 1
fi
if ! grep -q "Bun: expected >= $GUL_BUN_MIN_VERSION, found 1.4.1" "$fixture_dir/err"; then
  printf 'ERROR old Bun mismatch was not reported precisely\n' >&2
  cat "$fixture_dir/err" >&2
  exit 1
fi
if PATH="$fixture_dir:/usr/bin:/bin" "$checker" --bun-only >"$fixture_dir/out" 2>"$fixture_dir/err"; then
  printf 'ERROR unsupported old Bun unexpectedly passed --bun-only\n' >&2
  exit 1
fi
if ! grep -q "Bun: expected >= $GUL_BUN_MIN_VERSION, found 1.4.1" "$fixture_dir/err"; then
  printf 'ERROR old Bun --bun-only mismatch was not reported precisely\n' >&2
  cat "$fixture_dir/err" >&2
  exit 1
fi

write_command bun "$GUL_BUN_MIN_VERSION"

write_unavailable_command go
if PATH="$fixture_dir:$PATH" "$script_dir/../contract/generate.sh" "$fixture_dir/generated" >"$fixture_dir/out" 2>"$fixture_dir/err"; then
  printf 'ERROR contract generation accepted an unavailable Go command\n' >&2
  exit 1
fi
if ! grep -q "requires Go exactly $GUL_GO_VERSION on darwin/$GUL_TARGET_ARCH; found unavailable" "$fixture_dir/err"; then
  printf 'ERROR unavailable Go was not reported precisely\n' >&2
  cat "$fixture_dir/err" >&2
  exit 1
fi

write_command go "go version go$GUL_GO_VERSION linux/amd64"
if PATH="$fixture_dir:$PATH" "$script_dir/../contract/generate.sh" "$fixture_dir/generated" >"$fixture_dir/out" 2>"$fixture_dir/err"; then
  printf 'ERROR contract generation accepted a non-darwin/arm64 Go toolchain\n' >&2
  exit 1
fi
if ! grep -q "requires Go on darwin/$GUL_TARGET_ARCH; found linux/amd64" "$fixture_dir/err"; then
  printf 'ERROR contract generation platform gate did not fail precisely\n' >&2
  cat "$fixture_dir/err" >&2
  exit 1
fi

write_command go 'go version go1.26.6-rc.1 darwin/arm64'
if PATH="$fixture_dir:$PATH" "$script_dir/../contract/generate.sh" "$fixture_dir/generated" >"$fixture_dir/out" 2>"$fixture_dir/err"; then
  printf 'ERROR contract generation accepted a non-pinned Go prerelease\n' >&2
  exit 1
fi
if ! grep -q "requires Go exactly $GUL_GO_VERSION; found 1.26.6-rc.1" "$fixture_dir/err"; then
  printf 'ERROR contract generation Go prerelease gate did not fail precisely\n' >&2
  cat "$fixture_dir/err" >&2
  exit 1
fi

write_command go "go version go$GUL_GO_VERSION darwin/arm64"
write_unavailable_command protoc
if PATH="$fixture_dir:$PATH" "$script_dir/../contract/generate.sh" "$fixture_dir/generated" >"$fixture_dir/out" 2>"$fixture_dir/err"; then
  printf 'ERROR contract generation accepted an unavailable protoc command\n' >&2
  exit 1
fi
if ! grep -q "requires protoc >= $GUL_PROTOC_MIN_VERSION; found unavailable" "$fixture_dir/err"; then
  printf 'ERROR unavailable protoc was not reported precisely\n' >&2
  cat "$fixture_dir/err" >&2
  exit 1
fi

write_command protoc 'libprotoc 35.1-rc1'
if PATH="$fixture_dir:$PATH" "$script_dir/../contract/generate.sh" "$fixture_dir/generated" >"$fixture_dir/out" 2>"$fixture_dir/err"; then
  printf 'ERROR contract generation accepted a below-minimum protoc prerelease\n' >&2
  exit 1
fi
if ! grep -q "requires protoc >= $GUL_PROTOC_MIN_VERSION; found 35.1-rc1" "$fixture_dir/err"; then
  printf 'ERROR contract generation protoc prerelease gate did not fail precisely\n' >&2
  cat "$fixture_dir/err" >&2
  exit 1
fi

write_command protoc "libprotoc $GUL_PROTOC_MIN_VERSION"
write_unavailable_command buf
if PATH="$fixture_dir:$PATH" "$script_dir/../contract/generate.sh" "$fixture_dir/generated" >"$fixture_dir/out" 2>"$fixture_dir/err"; then
  printf 'ERROR contract generation accepted an unavailable Buf command\n' >&2
  exit 1
fi
if ! grep -q "requires Buf >= $GUL_BUF_MIN_VERSION; found unavailable" "$fixture_dir/err"; then
  printf 'ERROR unavailable Buf was not reported precisely\n' >&2
  cat "$fixture_dir/err" >&2
  exit 1
fi

write_command buf '1.66.1-rc.1'
if PATH="$fixture_dir:$PATH" "$script_dir/../contract/generate.sh" "$fixture_dir/generated" >"$fixture_dir/out" 2>"$fixture_dir/err"; then
  printf 'ERROR contract generation accepted a below-minimum Buf prerelease\n' >&2
  exit 1
fi
if ! grep -q "requires Buf >= $GUL_BUF_MIN_VERSION; found 1.66.1-rc.1" "$fixture_dir/err"; then
  printf 'ERROR contract generation Buf prerelease gate did not fail precisely\n' >&2
  cat "$fixture_dir/err" >&2
  exit 1
fi

write_command go 'go version go1.27rc1 darwin/arm64'
write_command protoc "libprotoc $GUL_PROTOC_MIN_VERSION"
if PATH="$fixture_dir:$PATH" "$script_dir/../contract/generate.sh" "$fixture_dir/generated" >"$fixture_dir/out" 2>"$fixture_dir/err"; then
  printf 'ERROR contract generation accepted a non-pinned newer Go prerelease\n' >&2
  exit 1
fi
if ! grep -q "requires Go exactly $GUL_GO_VERSION; found 1.27rc1" "$fixture_dir/err"; then
  printf 'ERROR exact Go gate did not reject a newer release candidate precisely\n' >&2
  cat "$fixture_dir/err" >&2
  exit 1
fi

write_command go "go version go$GUL_GO_VERSION darwin/arm64"
write_command protoc 'libprotoc 99.0.0'
write_command buf '1.0.0'
if PATH="$fixture_dir:$PATH" "$script_dir/../contract/generate.sh" "$fixture_dir/generated" >"$fixture_dir/out" 2>"$fixture_dir/err"; then
  printf 'ERROR contract generation unexpectedly passed with old Buf\n' >&2
  exit 1
fi
if ! grep -q "requires Buf >= $GUL_BUF_MIN_VERSION; found 1.0.0" "$fixture_dir/err"; then
  printf 'ERROR newer protoc did not pass through to the Buf gate\n' >&2
  cat "$fixture_dir/err" >&2
  exit 1
fi

write_command protoc "libprotoc $GUL_PROTOC_MIN_VERSION"
write_command buf '99.0.0'
if PATH="$fixture_dir:$PATH" "$script_dir/../contract/generate.sh" "$fixture_dir/generated" >"$fixture_dir/out" 2>"$fixture_dir/err"; then
  printf 'ERROR fixture generation unexpectedly passed its generator sentinel\n' >&2
  exit 1
fi
if ! grep -q "requires protoc-gen-go $GUL_PROTOC_GEN_GO_VERSION" "$fixture_dir/err"; then
  printf 'ERROR newer Buf did not pass through to the project-generator gate\n' >&2
  cat "$fixture_dir/err" >&2
  exit 1
fi

hermetic_root="$fixture_dir/hermetic-workspace"
hermetic_contract="$hermetic_root/contract"
hermetic_bin="$hermetic_root/bin"
mkdir -p "$hermetic_contract/node_modules/.bin" "$hermetic_contract/upstream/dolgorae/public/v1" \
  "$hermetic_contract/scripts" "$hermetic_root/scripts" "$hermetic_root/toolchain" "$hermetic_bin"
cp "$script_dir/../contract/generate.sh" "$hermetic_contract/generate.sh"
cp "$script_dir/../contract/go.mod" "$hermetic_contract/go.mod"
cp "$checker" "$hermetic_root/scripts/toolchain-check.sh"
cp "$manifest" "$hermetic_root/toolchain/versions.env"
printf 'pinned descriptor\n' > "$hermetic_contract/upstream/dolgorae-public-v1.descriptor.pb"
printf '{"generator":{"buf":"1.69.0"}}\n' > "$hermetic_contract/upstream/dolgorae-public-v1.descriptor.json"
printf 'syntax = "proto3";\n' > "$hermetic_contract/upstream/dolgorae/public/v1/dolgorae.proto"

cat > "$hermetic_bin/go" <<EOF
#!/bin/sh
if [ "\${1-}" = version ]; then
  printf '%s\n' 'go version go$GUL_GO_VERSION darwin/$GUL_TARGET_ARCH'
elif [ "\${1-}" = tool ] && [ "\${2-}" = protoc-gen-go ] && [ "\${3-}" = --version ]; then
  if [ "\${FIXTURE_PROTOC_GEN_GO_UNAVAILABLE:-0}" = 1 ]; then
    exit 127
  fi
  printf '%s\n' "\${FIXTURE_PROTOC_GEN_GO_VERSION:-protoc-gen-go v$GUL_PROTOC_GEN_GO_VERSION}"
elif [ "\${1-}" = tool ] && [ "\${2-}" = protoc-gen-connect-go ] && [ "\${3-}" = --version ]; then
  if [ "\${FIXTURE_PROTOC_GEN_CONNECT_GO_UNAVAILABLE:-0}" = 1 ]; then
    exit 127
  fi
  printf '%s\n' "\${FIXTURE_PROTOC_GEN_CONNECT_GO_VERSION:-$GUL_PROTOC_GEN_CONNECT_GO_VERSION}"
elif [ "\${1-}" = tool ] && [ "\${2-}" = protoc-gen-go ]; then
  printf '%s\n' protoc-gen-go >> '$hermetic_root/project-generator-invocations'
elif [ "\${1-}" = tool ] && [ "\${2-}" = protoc-gen-connect-go ]; then
  printf '%s\n' protoc-gen-connect-go >> '$hermetic_root/project-generator-invocations'
else
  exit 2
fi
EOF
cat > "$hermetic_bin/protoc" <<EOF
#!/bin/sh
if [ "\${1-}" = --version ]; then
  printf '%s\n' 'libprotoc $GUL_PROTOC_MIN_VERSION'
  exit 0
fi
go_output=
es_output=
for argument in "\$@"; do
  case "\$argument" in
    --go_out=*) go_output=\${argument#--go_out=} ;;
    --es_out=*) es_output=\${argument#--es_out=} ;;
  esac
done
protoc-gen-go </dev/null
protoc-gen-connect-go </dev/null
protoc-gen-es </dev/null
mkdir -p "\$go_output" "\$es_output"
printf 'package fixture\n' > "\$go_output/fixture.pb.go"
printf 'export {};\n' > "\$es_output/fixture_pb.ts"
EOF
cat > "$hermetic_bin/gofmt" <<'EOF'
#!/bin/sh
exit 0
EOF
cat > "$hermetic_bin/bun" <<EOF
#!/bin/sh
if [ "\${1-}" = --version ]; then
  printf '%s\n' '$GUL_BUN_MIN_VERSION'
fi
exit 0
EOF
cat > "$hermetic_bin/buf" <<'EOF'
#!/bin/sh
if [ "${1-}" = --version ]; then
  printf '%s\n' 99.0.0
  exit 0
fi
if [ "${1-}" = build ]; then
  shift
  while [ "$#" -gt 0 ]; do
    if [ "$1" = -o ]; then
      printf 'drifted descriptor\n' > "$2"
      exit 0
    fi
    shift
  done
  exit 2
fi
exit 0
EOF
cat > "$hermetic_contract/node_modules/.bin/protoc-gen-es" <<EOF
#!/bin/sh
if [ "\${1-}" = --version ]; then
  if [ "\${FIXTURE_PROTOC_GEN_ES_UNAVAILABLE:-0}" = 1 ]; then
    exit 127
  fi
  printf '%s\n' "\${FIXTURE_PROTOC_GEN_ES_VERSION:-protoc-gen-es v$GUL_PROTOC_GEN_ES_VERSION}"
else
  printf '%s\n' protoc-gen-es >> '$hermetic_root/project-generator-invocations'
fi
EOF
cat > "$hermetic_contract/breaking-check.sh" <<'EOF'
#!/bin/sh
exit 0
EOF
for generator in protoc-gen-go protoc-gen-connect-go protoc-gen-es; do
  cat > "$hermetic_bin/$generator" <<EOF
#!/bin/sh
printf '%s\n' invoked >> '$hermetic_root/path-generator-invocations'
exit 99
EOF
done
chmod +x "$hermetic_contract/generate.sh" "$hermetic_root/scripts/toolchain-check.sh" \
  "$hermetic_contract/breaking-check.sh" "$hermetic_contract/node_modules/.bin/protoc-gen-es" \
  "$hermetic_bin"/*

cp "$hermetic_contract/go.mod" "$hermetic_contract/go.mod.clean"
printf '\nreplace connectrpc.com/connect => ../untrusted-connect\n' >> "$hermetic_contract/go.mod"
if PATH="$hermetic_bin:/usr/bin:/bin" "$hermetic_contract/generate.sh" \
    "$hermetic_root/generated" >"$fixture_dir/out" 2>"$fixture_dir/err"; then
  printf 'ERROR contract generation accepted a replace directive\n' >&2
  exit 1
fi
if ! grep -q 'must not contain replace or exclude directives' "$fixture_dir/err"; then
  printf 'ERROR contract generation did not reject replace before Go execution\n' >&2
  cat "$fixture_dir/err" >&2
  exit 1
fi
mv "$hermetic_contract/go.mod.clean" "$hermetic_contract/go.mod"

expect_generator_rejection() {
  local variable=$1
  local value=$2
  local generator=$3
  local expected=$4
  if env "$variable=$value" PATH="$hermetic_bin:/usr/bin:/bin" \
    "$hermetic_contract/generate.sh" "$hermetic_root/generated" >"$fixture_dir/out" 2>"$fixture_dir/err"; then
    printf 'ERROR contract generation accepted %s version %s\n' "$generator" "$value" >&2
    exit 1
  fi
  if ! grep -q "requires $generator $expected; found $value" "$fixture_dir/err"; then
    printf 'ERROR %s mismatch was not reported precisely for %s\n' "$generator" "$value" >&2
    cat "$fixture_dir/err" >&2
    exit 1
  fi
}

expect_generator_rejection FIXTURE_PROTOC_GEN_GO_VERSION 'protoc-gen-go v1.36.11' \
  protoc-gen-go "$GUL_PROTOC_GEN_GO_VERSION"
expect_generator_rejection FIXTURE_PROTOC_GEN_GO_VERSION 'protoc-gen-go v1.36.13' \
  protoc-gen-go "$GUL_PROTOC_GEN_GO_VERSION"
expect_generator_rejection FIXTURE_PROTOC_GEN_CONNECT_GO_VERSION '1.19.9' \
  protoc-gen-connect-go "$GUL_PROTOC_GEN_CONNECT_GO_VERSION"
expect_generator_rejection FIXTURE_PROTOC_GEN_CONNECT_GO_VERSION '1.20.1' \
  protoc-gen-connect-go "$GUL_PROTOC_GEN_CONNECT_GO_VERSION"
expect_generator_rejection FIXTURE_PROTOC_GEN_ES_VERSION 'protoc-gen-es v2.13.9' \
  protoc-gen-es "$GUL_PROTOC_GEN_ES_VERSION"
expect_generator_rejection FIXTURE_PROTOC_GEN_ES_VERSION 'protoc-gen-es v2.14.1' \
  protoc-gen-es "$GUL_PROTOC_GEN_ES_VERSION"

expect_generator_unavailable() {
  local variable=$1
  local generator=$2
  local expected=$3
  if env "$variable=1" PATH="$hermetic_bin:/usr/bin:/bin" \
    "$hermetic_contract/generate.sh" "$hermetic_root/generated" >"$fixture_dir/out" 2>"$fixture_dir/err"; then
    printf 'ERROR contract generation accepted unavailable %s\n' "$generator" >&2
    exit 1
  fi
  if ! grep -q "requires $generator $expected; found unavailable" "$fixture_dir/err"; then
    printf 'ERROR unavailable %s was not reported precisely\n' "$generator" >&2
    cat "$fixture_dir/err" >&2
    exit 1
  fi
}

expect_generator_unavailable FIXTURE_PROTOC_GEN_GO_UNAVAILABLE \
  protoc-gen-go "$GUL_PROTOC_GEN_GO_VERSION"
expect_generator_unavailable FIXTURE_PROTOC_GEN_CONNECT_GO_UNAVAILABLE \
  protoc-gen-connect-go "$GUL_PROTOC_GEN_CONNECT_GO_VERSION"
expect_generator_unavailable FIXTURE_PROTOC_GEN_ES_UNAVAILABLE \
  protoc-gen-es "$GUL_PROTOC_GEN_ES_VERSION"

chmod -x "$hermetic_contract/node_modules/.bin/protoc-gen-es"
if PATH="$hermetic_bin:/usr/bin:/bin" "$hermetic_contract/generate.sh" \
  "$hermetic_root/generated" >"$fixture_dir/out" 2>"$fixture_dir/err"; then
  printf 'ERROR contract generation accepted a missing project protoc-gen-es\n' >&2
  exit 1
fi
if ! grep -q 'missing pinned contract dependency; run make test-prepare' "$fixture_dir/err"; then
  printf 'ERROR missing project protoc-gen-es was not reported precisely\n' >&2
  cat "$fixture_dir/err" >&2
  exit 1
fi
chmod +x "$hermetic_contract/node_modules/.bin/protoc-gen-es"

if PATH="$hermetic_bin:/usr/bin:/bin" "$hermetic_contract/generate.sh" "$hermetic_root/generated" \
  >"$fixture_dir/out" 2>"$fixture_dir/err"; then
  printf 'ERROR newer Buf descriptor drift unexpectedly passed generation\n' >&2
  exit 1
fi
if ! grep -q 'reproduced descriptor differs from the pinned TASK-053 descriptor' "$fixture_dir/err"; then
  printf 'ERROR newer Buf descriptor drift was not rejected precisely\n' >&2
  cat "$fixture_dir/err" >&2
  exit 1
fi
if [ -e "$hermetic_root/path-generator-invocations" ]; then
  printf 'ERROR contract generation invoked a PATH generator instead of project-owned generators\n' >&2
  exit 1
fi

cat > "$hermetic_bin/buf" <<'EOF'
#!/bin/sh
if [ "${1-}" = --version ]; then
  printf '%s\n' 99.0.0
  exit 0
fi
if [ "${1-}" = build ]; then
  shift
  while [ "$#" -gt 0 ]; do
    if [ "$1" = -o ]; then
      cp upstream/dolgorae-public-v1.descriptor.pb "$2"
      exit 0
    fi
    shift
  done
  exit 2
fi
exit 0
EOF
chmod +x "$hermetic_bin/buf"
rm -f "$hermetic_root/project-generator-invocations" \
  "$hermetic_root/path-generator-invocations"
PATH="$hermetic_bin:/usr/bin:/bin" "$hermetic_contract/generate.sh" \
  "$hermetic_root/generated" >"$fixture_dir/out" 2>"$fixture_dir/err"
for generator in protoc-gen-go protoc-gen-connect-go protoc-gen-es; do
  if ! grep -qx "$generator" "$hermetic_root/project-generator-invocations"; then
    printf 'ERROR successful generation did not invoke project-owned %s\n' "$generator" >&2
    exit 1
  fi
done
if [ -e "$hermetic_root/path-generator-invocations" ]; then
  printf 'ERROR successful generation invoked a PATH generator\n' >&2
  exit 1
fi

write_command go "go version go$GUL_GO_VERSION darwin/arm64"
write_command buf 1.69.0
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

write_command bun 1.4.1
if PATH="$fixture_dir:$PATH" make -C "$script_dir/.." test-prepare >"$fixture_dir/out" 2>"$fixture_dir/err"; then
  printf 'ERROR test-prepare accepted an old system Bun\n' >&2
  exit 1
fi
if ! grep -q "Bun: expected >= $GUL_BUN_MIN_VERSION, found 1.4.1" "$fixture_dir/err"; then
  printf 'ERROR test-prepare did not stop at the Bun minimum gate\n' >&2
  exit 1
fi
if PATH="$fixture_dir:$PATH" "$script_dir/../contract/generate.sh" "$fixture_dir/generated" >"$fixture_dir/out" 2>"$fixture_dir/err"; then
  printf 'ERROR contract generation accepted an old system Bun\n' >&2
  exit 1
fi
if ! grep -q "Bun: expected >= $GUL_BUN_MIN_VERSION, found 1.4.1" "$fixture_dir/err"; then
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

write_command wails3 v3.0.0-beta.7
if PATH="$fixture_dir:/usr/bin:/bin" "$checker" >"$fixture_dir/out" 2>"$fixture_dir/err"; then
  printf 'ERROR mismatched Wails unexpectedly passed\n' >&2
  exit 1
fi
if ! grep -q "Wails: expected >= $GUL_WAILS_MIN_VERSION, found 3.0.0-beta.7" "$fixture_dir/err"; then
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

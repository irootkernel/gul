#!/usr/bin/env bash
set -u

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
manifest="$script_dir/../toolchain/versions.env"

if [ ! -r "$manifest" ]; then
  printf 'ERROR manifest missing: %s\n' "$manifest" >&2
  exit 2
fi

if ! awk '
  /^[[:space:]]*($|#)/ { next }
  /^GUL_[A-Z0-9_]+=[0-9A-Za-z.+-]+$/ { next }
  { bad = 1; print "ERROR invalid manifest line " NR ": " $0 > "/dev/stderr" }
  END { exit bad }
' "$manifest"; then
  exit 2
fi

# shellcheck disable=SC1090
. "$manifest"

required_keys='GUL_GO_VERSION GUL_WAILS_VERSION GUL_NODE_VERSION GUL_BUN_MIN_VERSION GUL_TYPESCRIPT_VERSION GUL_REACT_VERSION GUL_REACT_DOM_VERSION GUL_BUF_MIN_VERSION GUL_BUF_MAX_EXCLUSIVE_VERSION GUL_PROTOC_VERSION GUL_PROTOC_GEN_GO_VERSION GUL_PROTOC_GEN_CONNECT_GO_VERSION GUL_PROTOBUF_GO_VERSION GUL_CONNECT_GO_VERSION GUL_CONNECT_ES_VERSION GUL_CONNECT_WEB_VERSION GUL_PROTOBUF_ES_VERSION GUL_PROTOC_GEN_ES_VERSION GUL_PROTOC_GEN_CONNECT_ES_VERSION GUL_MODERNC_SQLITE_VERSION GUL_GIT_MIN_VERSION GUL_GIT_MAX_EXCLUSIVE_VERSION GUL_MACOS_MIN_VERSION GUL_TARGET_ARCH'
for key in $required_keys; do
  eval "value=\${$key-}"
  if [ -z "$value" ]; then
    printf 'ERROR required manifest key is empty: %s\n' "$key" >&2
    exit 2
  fi
done

if [ "${1-}" = "--manifest-only" ]; then
  printf 'OK manifest: %s\n' "$manifest"
  exit 0
fi
if [ "${1-}" != "" ] && [ "${1-}" != "--bun-only" ]; then
  printf 'usage: %s [--manifest-only|--bun-only]\n' "$0" >&2
  exit 2
fi

failures=0

fail() {
  printf 'FAIL %s\n' "$1" >&2
  failures=$((failures + 1))
}

pass() {
  printf 'OK   %s\n' "$1"
}

command_output() {
  local command_name=$1
  local gul_command_output
  local gul_command_rc
  shift
  if ! command -v "$command_name" >/dev/null 2>&1; then
    return 127
  fi
  gul_command_output=$("$command_name" "$@" 2>&1)
  gul_command_rc=$?
  if [ "$gul_command_rc" -ne 0 ]; then
    return "$gul_command_rc"
  fi
  printf '%s\n' "$gul_command_output" | awk 'NR == 1 { print; exit }'
}

check_exact() {
  local label=$1
  local expected=$2
  local command_name=$3
  local output
  local gul_exact_rc
  local actual
  shift 3
  output=$(command_output "$command_name" "$@")
  gul_exact_rc=$?
  if [ "$gul_exact_rc" -ne 0 ]; then
    fail "$label: missing command '$command_name'"
    return
  fi
  actual=$(printf '%s\n' "$output" | sed -E 's/^go version go//; s/^go//; s/^v//; s/^libprotoc[[:space:]]+//; s/^[^0-9]*//; s/[[:space:]].*$//')
  if [ "$actual" != "$expected" ]; then
    fail "$label: expected $expected, found $actual ($command_name)"
    return
  fi
  pass "$label $actual"
}

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

check_range() {
  local label=$1
  local minimum=$2
  local maximum_exclusive=$3
  local command_name=$4
  local output
  local gul_range_rc
  local actual
  shift 4
  output=$(command_output "$command_name" "$@")
  gul_range_rc=$?
  if [ "$gul_range_rc" -ne 0 ]; then
    fail "$label: missing command '$command_name'"
    return
  fi
  actual=$(printf '%s\n' "$output" | sed -E 's/^v//; s/^[^0-9]*//; s/[[:space:]].*$//')
  if version_cmp "$actual" ge "$minimum" && version_cmp "$actual" lt "$maximum_exclusive"; then
    pass "$label $actual (>= $minimum, < $maximum_exclusive)"
  else
    fail "$label: expected >= $minimum and < $maximum_exclusive, found $actual ($command_name)"
  fi
}

check_minimum() {
  local label=$1
  local minimum=$2
  local command_name=$3
  local output
  local gul_minimum_rc
  local actual
  shift 3
  output=$(command_output "$command_name" "$@")
  gul_minimum_rc=$?
  if [ "$gul_minimum_rc" -ne 0 ]; then
    fail "$label: missing command '$command_name'"
    return
  fi
  actual=$(printf '%s\n' "$output" | sed -E 's/^v//; s/^[^0-9]*//; s/[[:space:]].*$//')
  if version_cmp "$actual" ge "$minimum"; then
    pass "$label $actual (>= $minimum)"
  else
    fail "$label: expected >= $minimum, found $actual ($command_name)"
  fi
}

if [ "${1-}" = "--bun-only" ]; then
  check_minimum Bun "$GUL_BUN_MIN_VERSION" bun --version
  if [ "$failures" -ne 0 ]; then
    printf 'toolchain check failed: %s issue(s)\n' "$failures" >&2
    exit 1
  fi
  exit 0
fi

os_name=$(command_output uname -s)
if [ "$os_name" = "Darwin" ]; then
  pass "host OS Darwin"
else
  fail "host OS: expected Darwin, found ${os_name:-unknown}"
fi

arch=$(command_output uname -m)
if [ "$arch" = "$GUL_TARGET_ARCH" ]; then
  pass "host architecture $arch"
else
  fail "host architecture: expected $GUL_TARGET_ARCH, found ${arch:-unknown}"
fi

macos=$(command_output sw_vers -productVersion)
if [ $? -ne 0 ]; then
  fail "macOS version: missing command 'sw_vers'"
elif version_cmp "$macos" ge "$GUL_MACOS_MIN_VERSION"; then
  pass "macOS $macos (>= $GUL_MACOS_MIN_VERSION)"
else
  fail "macOS: expected >= $GUL_MACOS_MIN_VERSION, found $macos"
fi

check_exact Go "$GUL_GO_VERSION" go version
check_exact Wails "$GUL_WAILS_VERSION" wails3 version
check_exact Node "$GUL_NODE_VERSION" node --version
check_minimum Bun "$GUL_BUN_MIN_VERSION" bun --version
check_range Buf "$GUL_BUF_MIN_VERSION" "$GUL_BUF_MAX_EXCLUSIVE_VERSION" buf --version
check_exact protoc "$GUL_PROTOC_VERSION" protoc --version
check_exact protoc-gen-go "$GUL_PROTOC_GEN_GO_VERSION" protoc-gen-go --version
check_exact protoc-gen-connect-go "$GUL_PROTOC_GEN_CONNECT_GO_VERSION" protoc-gen-connect-go --version
check_exact protoc-gen-es "$GUL_PROTOC_GEN_ES_VERSION" protoc-gen-es --version
check_exact protoc-gen-connect-es "$GUL_PROTOC_GEN_CONNECT_ES_VERSION" protoc-gen-connect-es --version

git_line=$(command_output git --version)
if [ $? -ne 0 ]; then
  fail "Git: missing command 'git'"
else
  git_version=$(printf '%s\n' "$git_line" | sed -E 's/^git version[[:space:]]+//; s/[[:space:]].*$//')
  if version_cmp "$git_version" ge "$GUL_GIT_MIN_VERSION" && version_cmp "$git_version" lt "$GUL_GIT_MAX_EXCLUSIVE_VERSION"; then
    pass "Git $git_version (>= $GUL_GIT_MIN_VERSION, < $GUL_GIT_MAX_EXCLUSIVE_VERSION)"
  else
    fail "Git: expected >= $GUL_GIT_MIN_VERSION and < $GUL_GIT_MAX_EXCLUSIVE_VERSION, found $git_version"
  fi
fi

if [ "$failures" -ne 0 ]; then
  printf 'toolchain check failed: %s issue(s)\n' "$failures" >&2
  exit 1
fi

printf 'toolchain check passed\n'

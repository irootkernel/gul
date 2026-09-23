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

required_keys='GUL_GO_VERSION GUL_WAILS_MIN_VERSION GUL_WAILS_GO_VERSION GUL_NODE_MIN_VERSION GUL_BUN_MIN_VERSION GUL_TYPESCRIPT_VERSION GUL_REACT_VERSION GUL_REACT_DOM_VERSION GUL_REACT_TYPES_VERSION GUL_REACT_DOM_TYPES_VERSION GUL_BUN_TYPES_VERSION GUL_BUF_MIN_VERSION GUL_PROTOC_MIN_VERSION GUL_PROTOC_GEN_GO_VERSION GUL_PROTOC_GEN_CONNECT_GO_VERSION GUL_PROTOBUF_GO_VERSION GUL_CONNECT_GO_VERSION GUL_CONNECT_ES_VERSION GUL_CONNECT_WEB_VERSION GUL_PROTOBUF_ES_VERSION GUL_PROTOC_GEN_ES_VERSION GUL_MODERNC_SQLITE_VERSION GUL_GIT_MIN_VERSION GUL_MACOS_MIN_VERSION GUL_TARGET_ARCH'
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
if [ "${1-}" != "" ] && [ "${1-}" != "--go-only" ] && [ "${1-}" != "--bun-only" ] && [ "${1-}" != "--api-only" ] && [ "${1-}" != "--version-at-least" ]; then
  printf 'usage: %s [--manifest-only|--go-only|--bun-only|--api-only|--version-at-least <actual> <minimum>]\n' "$0" >&2
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

version_cmp() {
  local left=$1
  local op=$2
  local right=$3
  awk -v left="$left" -v right="$right" -v op="$op" '
    function compare(left_value, right_value, left_main, right_main, left_pre, right_pre, left_parts, right_parts, left_count, right_count, count, i, a, b, a_numeric, b_numeric) {
      sub(/[+].*$/, "", left_value)
      sub(/[+].*$/, "", right_value)
      left_main = left_value
      right_main = right_value
      left_pre = ""
      right_pre = ""
      if (index(left_main, "-") > 0) {
        left_pre = substr(left_main, index(left_main, "-") + 1)
        left_main = substr(left_main, 1, index(left_main, "-") - 1)
      }
      if (index(right_main, "-") > 0) {
        right_pre = substr(right_main, index(right_main, "-") + 1)
        right_main = substr(right_main, 1, index(right_main, "-") - 1)
      }
      left_count = split(left_main, left_parts, /[.]/)
      right_count = split(right_main, right_parts, /[.]/)
      count = left_count > right_count ? left_count : right_count
      for (i = 1; i <= count; i++) {
        if ((i <= left_count && left_parts[i] !~ /^[0-9]+$/) || (i <= right_count && right_parts[i] !~ /^[0-9]+$/)) exit 2
        a = i <= left_count ? left_parts[i] + 0 : 0
        b = i <= right_count ? right_parts[i] + 0 : 0
        if (a < b) return -1
        if (a > b) return 1
      }
      if (left_pre == "" && right_pre == "") return 0
      if (left_pre == "") return 1
      if (right_pre == "") return -1
      left_count = split(left_pre, left_parts, /[.]/)
      right_count = split(right_pre, right_parts, /[.]/)
      count = left_count > right_count ? left_count : right_count
      for (i = 1; i <= count; i++) {
        if (i > left_count) return -1
        if (i > right_count) return 1
        a = left_parts[i]
        b = right_parts[i]
        a_numeric = a ~ /^[0-9]+$/
        b_numeric = b ~ /^[0-9]+$/
        if (a_numeric && b_numeric) {
          if (a + 0 < b + 0) return -1
          if (a + 0 > b + 0) return 1
        } else if (a_numeric != b_numeric) {
          return a_numeric ? -1 : 1
        } else {
          if (a < b) return -1
          if (a > b) return 1
        }
      }
      return 0
    }
    BEGIN {
      cmp = compare(left, right)
      if (op == "ge") exit !(cmp >= 0)
      if (op == "lt") exit !(cmp < 0)
      exit 2
    }
  '
}

if [ "${1-}" = "--version-at-least" ]; then
  if [ "$#" -ne 3 ]; then
    printf 'usage: %s --version-at-least <actual> <minimum>\n' "$0" >&2
    exit 2
  fi
  version_cmp "$2" ge "$3"
  exit $?
fi

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
  actual=$(printf '%s\n' "$output" | sed -E 's/^v//; s/^[^0-9]*//; s/[[:space:]].*$//')
  if [ "$actual" = "$expected" ]; then
    pass "$label $actual (exact)"
  else
    fail "$label: expected exactly $expected, found $actual ($command_name)"
  fi
}

if [ "${1-}" = "--go-only" ]; then
  check_exact Go "$GUL_GO_VERSION" go version
  if [ "$failures" -ne 0 ]; then
    printf 'toolchain check failed: %s issue(s)\n' "$failures" >&2
    exit 1
  fi
  exit 0
fi

if [ "${1-}" = "--bun-only" ]; then
  check_minimum Bun "$GUL_BUN_MIN_VERSION" bun --version
  if [ "$failures" -ne 0 ]; then
    printf 'toolchain check failed: %s issue(s)\n' "$failures" >&2
    exit 1
  fi
  exit 0
fi

if [ "${1-}" = "--api-only" ]; then
  check_exact Go "$GUL_GO_VERSION" go version
  if command_output gofmt -h >/dev/null; then
    pass "gofmt available"
  else
    fail "gofmt: missing command 'gofmt'"
  fi
  check_minimum Node "$GUL_NODE_MIN_VERSION" node --version
  check_minimum Bun "$GUL_BUN_MIN_VERSION" bun --version
  check_minimum Buf "$GUL_BUF_MIN_VERSION" buf --version
  check_minimum protoc "$GUL_PROTOC_MIN_VERSION" protoc --version
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
check_minimum Wails "$GUL_WAILS_MIN_VERSION" wails3 version
check_minimum Node "$GUL_NODE_MIN_VERSION" node --version
check_minimum Bun "$GUL_BUN_MIN_VERSION" bun --version
check_minimum Buf "$GUL_BUF_MIN_VERSION" buf --version
check_minimum protoc "$GUL_PROTOC_MIN_VERSION" protoc --version

git_line=$(command_output git --version)
if [ $? -ne 0 ]; then
  fail "Git: missing command 'git'"
else
  git_version=$(printf '%s\n' "$git_line" | sed -E 's/^git version[[:space:]]+//; s/[[:space:]].*$//')
  if version_cmp "$git_version" ge "$GUL_GIT_MIN_VERSION"; then
    pass "Git $git_version (>= $GUL_GIT_MIN_VERSION)"
  else
    fail "Git: expected >= $GUL_GIT_MIN_VERSION, found $git_version"
  fi
fi

if [ "$failures" -ne 0 ]; then
  printf 'toolchain check failed: %s issue(s)\n' "$failures" >&2
  exit 1
fi

printf 'toolchain check passed\n'

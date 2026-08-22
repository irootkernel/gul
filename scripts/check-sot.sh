#!/usr/bin/env bash
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/.." && pwd)
docs='architecture-decision-records.md architecture.md implementation-memo.md required-specs.md roadmap.md'

for name in $docs; do
  if [ ! -r "$repo_root/docs/$name" ]; then
    printf 'ERROR missing SOT document: docs/%s\n' "$name" >&2
    exit 1
  fi
done

task_count=$(awk '/^#### E[0-9]+-T[0-9]+ / { count++ } END { print count + 0 }' "$repo_root/docs/roadmap.md")
retired_count=$(awk '
  /^#### E[0-9]+-T[0-9]+ / { task = 1; next }
  task && /^\*\*Status:\*\*/ { if ($0 ~ /`Retired`/) retired++; task = 0 }
  END { print retired + 0 }
' "$repo_root/docs/roadmap.md")
active_count=$(awk '
  /^#### E[0-9]+-T[0-9]+ / { task = 1; next }
  task && /^\*\*Status:\*\*/ { if ($0 ~ /`In Progress`|`In Review`/) active++; task = 0 }
  END { print active + 0 }
' "$repo_root/docs/roadmap.md")

if [ "$task_count" -ne 37 ] || [ "$retired_count" -ne 1 ]; then
  printf 'ERROR roadmap expected 36 executable Tasks plus one retired Task; found %s total and %s retired\n' "$task_count" "$retired_count" >&2
  exit 1
fi
if [ "$active_count" -ne 1 ]; then
  printf 'ERROR roadmap expected exactly one active Task; found %s\n' "$active_count" >&2
  exit 1
fi

fixture_dir=$(mktemp -d "${TMPDIR:-/tmp}/gul-sot.XXXXXX")
cleanup() { rm -rf "$fixture_dir"; }
trap cleanup EXIT HUP INT TERM

awk -F'|' '
  /^### 5\.1 / { ledger = 1 }
  /^## 6\./ { ledger = 0 }
  ledger && /^\| REQ-[A-Z0-9-]+ / { value=$2; gsub(/[[:space:]]/, "", value); print value }
' "$repo_root/docs/required-specs.md" > "$fixture_dir/requirements"
if [ "$(wc -l < "$fixture_dir/requirements" | tr -d ' ')" -ne "$(sort -u "$fixture_dir/requirements" | wc -l | tr -d ' ')" ]; then
  printf 'ERROR duplicate requirement definition\n' >&2
  exit 1
fi

awk -F'|' '/^\| ADR-[0-9][0-9][0-9][0-9] / { value=$2; gsub(/[[:space:]]/, "", value); print value }' "$repo_root/docs/architecture-decision-records.md" | sort -u > "$fixture_dir/adr-index"
sed -nE 's/^### (ADR-[0-9]{4}):.*/\1/p' "$repo_root/docs/architecture-decision-records.md" | sort -u > "$fixture_dir/adr-body"
if ! cmp -s "$fixture_dir/adr-index" "$fixture_dir/adr-body"; then
  printf 'ERROR ADR index and bodies differ\n' >&2
  diff -u "$fixture_dir/adr-index" "$fixture_dir/adr-body" >&2 || true
  exit 1
fi

printf 'SOT checks passed: %s executable Tasks, %s active, %s requirements, %s ADRs\n' \
  "$((task_count - retired_count))" "$active_count" \
  "$(wc -l < "$fixture_dir/requirements" | tr -d ' ')" \
  "$(wc -l < "$fixture_dir/adr-index" | tr -d ' ')"

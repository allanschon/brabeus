#!/usr/bin/env bash
# Regression test for refcheck.sh: a reference a reader cannot follow fails the
# check and is named; a tree with none passes. Builds throwaway repositories.
#
# Usage: scripts/refcheck.test.sh [path-to-refcheck]
set -uo pipefail
check="${1:-$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/refcheck.sh}"
failed=0
repo() { # repo <dir>: an empty git repository with the top-level directories refcheck knows
  git init -q "$1" && mkdir -p "$1/docs/sub" "$1/internal/pkg"
}
expect() { # expect <pass|fail> <description> <dir> [text the output must contain]
  local want=$1 what=$2 dir=$3 needle=${4:-} out got
  git -C "$dir" add -A >/dev/null
  out=$("$check" "$dir" 2>&1) && got=pass || got=fail
  if [ "$got" != "$want" ] || { [ -n "$needle" ] && ! grep -qF -- "$needle" <<<"$out"; }; then
    echo "FAIL $what: want $want${needle:+ naming '$needle'}, got $got"; printf '%s\n' "$out" | sed 's/^/     /'; failed=$((failed+1))
  else
    echo "ok   $what"
  fi
}
tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT

repo "$tmp/clean"
printf 'package pkg\n// See docs/guide.md and internal/pkg/pkg.go, and internal/claims once it exists.\n' > "$tmp/clean/internal/pkg/pkg.go"
printf '# Guide\n\n[sub](sub/page.md#part) and [up](../README.md) and [web](https://example.org/x.md).\n' > "$tmp/clean/docs/guide.md"
printf '# Page\n' > "$tmp/clean/docs/sub/page.md"
printf '# Readme\n' > "$tmp/clean/README.md"
expect pass "a tree whose references all resolve" "$tmp/clean"

repo "$tmp/label"
printf 'package pkg\n// The alias stays for one milestone (decision D6).\n' > "$tmp/label/internal/pkg/pkg.go"
expect fail "a decision cited by label" "$tmp/label" "internal/pkg/pkg.go:2"

repo "$tmp/path"
printf 'package pkg\n// Reasoning: docs/plans/2026-01-01-private.md, Task 4.\n' > "$tmp/path/internal/pkg/pkg.go"
expect fail "a path to a file not in the tree" "$tmp/path" "docs/plans/2026-01-01-private.md"

repo "$tmp/link"
printf '# Guide\n\nSee [the plan](plans/missing.md).\n' > "$tmp/link/docs/guide.md"
expect fail "a markdown link to a file not in the tree" "$tmp/link" "plans/missing.md"

echo; echo "$failed failed"
[ "$failed" -eq 0 ]

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

# Planning labels in their other forms: each fails on its own.
n=0
for text in '// Scope is checked without a project (K14).' '// K10: commit only on a change.' \
            '// Task 3'"'"'s review found this.' '// The finding this task fixes.' \
            '// Only claims is a block (ruling on the plan).' '// Settled in the second review round.'; do
  n=$((n+1)); repo "$tmp/plan$n"
  printf 'package pkg\n%s\n' "$text" > "$tmp/plan$n/internal/pkg/pkg.go"
  expect fail "a planning label: $text" "$tmp/plan$n" "internal/pkg/pkg.go:2"
done

# What a reader can follow passes: milestones, goal ids, spec change rows, and
# "ruling out" as ordinary English.
repo "$tmp/defined"
printf 'package pkg\n// The block (M2) renders G1 (G1) as spec §16 AV says.\n// M1: enforced since then.\n// Ruling out a stale pass is the point.\n' > "$tmp/defined/internal/pkg/pkg.go"
printf '# Intent (M2)\n\nRow AX of §16 changed it.\n' > "$tmp/defined/docs/guide.md"
expect pass "milestones, goal ids, change rows and ordinary words" "$tmp/defined"

echo; echo "$failed failed"
[ "$failed" -eq 0 ]

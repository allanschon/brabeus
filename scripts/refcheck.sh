#!/usr/bin/env bash
# Fails on a reference a reader of this repository cannot follow.
#
# WHY. The repository is public and its reasoning has to be too. A comment that
#    justifies code with a label defined somewhere else, or points at a file that
#    is not in the tree, tells the reader there is a reason without giving it. The
#    leak check cannot catch these: it refuses words that must not appear, and
#    these references are made of ordinary words.
#
# Three rules:
#   1. no reasoning cited by a planning label: "decision D6", a bare label in
#      brackets ("(K14)") or opening a comment ("// K10:"), "Task 3", "this
#      task", a ruling, or a fix or review round. State the reason, or cite the
#      spec section that records it. Milestones (M0, M1, ...) and goal ids (G1,
#      ...) are defined in the specification and the record, so they pass, and
#      so do the specification's change rows, which are letters only ("§16 AV")
#   2. a path under a top-level directory of this repository, ending in a file
#      extension, names a file that exists. Directories are not checked, so a
#      package a later milestone will add can be named before it exists
#   3. a relative markdown link resolves to a file that exists
#
# Usage: scripts/refcheck.sh [repository root]
set -euo pipefail
cd "${1:-$(git -c safe.directory='*' rev-parse --show-toplevel)}"
g() { git -c safe.directory='*' "$@"; }
self=':!scripts/refcheck.sh'
selftest=':!scripts/refcheck.test.sh'
hits=()

while IFS= read -r line; do
  hits+=("$line  (a decision cited by label: state the reason instead)")
done < <(g grep -n -I -E '\bdecisions? [A-Z][0-9]+\b' -- . "$self" "$selftest" || true)

# A label is one letter and one or two digits. M (milestones) and G (goal ids)
# are left out, because both are defined where a reader can find them.
label='[A-FH-LN-Z][0-9]{1,2}'
while IFS= read -r line; do
  hits+=("$line  (a planning label: state the reason instead)")
done < <(g grep -n -I -E "\\($label\\)|(//|#)[[:space:]]*$label\\b|\\bTask [0-9]+\\b|\\b[Tt]his task\\b|\\b([Ff]ix|[Rr]eview) rounds?\\b|\\b[Rr]ulings?\\b" -- . "$self" "$selftest" \
  | grep -v -E '\b[Rr]uling out\b' || true)

dirs='cmd|internal|docs|modules|plugins|scripts'
exts='md|go|sh|py|json|tmpl|yml|yaml'
while IFS= read -r m; do
  file=${m%%:*}; rest=${m#*:}; num=${rest%%:*}; path=${rest#*:}
  path=${path#[^A-Za-z0-9_.]}
  [ -e "$path" ] || hits+=("$file:$num: $path  (no such file in this repository)")
done < <(g grep -n -o -I -E "(^|[[:space:]\`\"'(=:\\[])($dirs)/[A-Za-z0-9_./-]+\\.($exts)\\b" -- . "$self" "$selftest" || true)

while IFS= read -r m; do
  file=${m%%:*}; rest=${m#*:}; num=${rest%%:*}; link=${rest#*:}
  target=${link#*](}; target=${target%)}; target=${target%%#*}
  case "$target" in ''|http://*|https://*|mailto:*|/*) continue ;; esac
  [ -e "$(dirname "$file")/$target" ] || hits+=("$file:$num: $target  (link to a file not in this repository)")
done < <(g grep -n -o -I -E '\]\([^)[:space:]]+\)' -- '*.md' || true)

if [ ${#hits[@]} -gt 0 ]; then
  printf '%s\n' "${hits[@]}"
  echo "refcheck: ${#hits[@]} reference(s) a reader cannot follow"
  exit 1
fi
echo "refcheck: clean"

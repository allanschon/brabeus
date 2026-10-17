#!/usr/bin/env bash
# Tests .githooks' call to a person's own check: absent, refusing and passing, for both
# pre-commit and commit-msg. Each case commits in a throwaway repository wired to this
# tree's hooks, with XDG_CONFIG_HOME pointed at a throwaway config directory.
set -euo pipefail
hooks="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.githooks" && pwd)"
t="$(mktemp -d)"; trap 'rm -rf "$t"' EXIT
export XDG_CONFIG_HOME="$t/config"
git init -q "$t/repo"
cd "$t/repo"
git config core.hooksPath "$hooks"
git config commit.gpgsign false
git config user.name test
git config user.email test@example.invalid

n=0
commit() { n=$((n+1)); echo "$n" > f; git add f; git commit -q -m "$1" >/dev/null 2>&1; }
local_hook() { mkdir -p "$XDG_CONFIG_HOME/brabeus/hooks"; printf '#!/bin/sh\n%s\n' "$2" > "$XDG_CONFIG_HOME/brabeus/hooks/$1"; chmod +x "$XDG_CONFIG_HOME/brabeus/hooks/$1"; }
fail() { echo "githooks.test: $1" >&2; exit 1; }

commit "no local hooks" || fail "a commit with no local hooks was refused"

local_hook pre-commit 'exit 1'
commit "pre-commit refuses" && fail "a refusing local pre-commit did not refuse the commit"
local_hook pre-commit 'exit 0'
commit "pre-commit passes" || fail "a passing local pre-commit refused the commit"

local_hook commit-msg 'grep -q forbidden "$1" && exit 1; exit 0'
commit "this message is forbidden" && fail "a refusing local commit-msg did not refuse the commit"
commit "this message is fine" || fail "a passing local commit-msg refused the commit"

# A check that is there but cannot run, such as a link left dangling when its target moved,
# must refuse rather than be skipped: skipping it would end the check with nothing said.
for h in pre-commit commit-msg; do
  rm -f "$XDG_CONFIG_HOME/brabeus/hooks/$h"
  ln -s "$t/moved-away" "$XDG_CONFIG_HOME/brabeus/hooks/$h"
  commit "dangling $h" && fail "a dangling local $h did not refuse the commit"
  rm -f "$XDG_CONFIG_HOME/brabeus/hooks/$h"
done

echo "githooks.test: ok"

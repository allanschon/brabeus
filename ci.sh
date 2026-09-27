#!/usr/bin/env bash
# The gates. The ONE definition of what "the tests pass" means.
#
# Two entry points call this and must never disagree:
#   test.sh                     - by hand and from .githooks/pre-push, in a container
#   .github/workflows/ci.yml   - in CI, already inside golang:1.25
#
# GOFLAGS and the "$@" pass-through live HERE, not in the caller. Left in test.sh,
#    CI would silently run -mod=readonly and could not take a -run filter - two entry
#    points running different tests, which is what this split exists to prevent.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")"
export GOFLAGS=-mod=mod

unformatted=$(gofmt -l .)
if [ -n "$unformatted" ]; then echo "gofmt would change:"; echo "$unformatted"; exit 1; fi

go vet -buildvcs=false ./...
go test -buildvcs=false ./... "$@"

# Build LAST but still gate on it: vet and test do not link a binary, so a package
# with no func main passes both while producing no server at all. That false green
# let an unrunnable commit through on 2026-09-06.
go build -buildvcs=false -o /dev/null ./cmd/brabeus

# Every reference in the tree must be one a reader can follow: no decision cited
# by a label defined elsewhere, no path to a file that is not here. The test runs
# first so a broken check cannot pass the tree by accident.
bash scripts/refcheck.test.sh >/dev/null
./scripts/refcheck.sh

# ---------------------------------------------------------------------------
# Backstop: did a plugin's content change without its version changing?
#
# A Claude Code plugin's cache is keyed by the version in plugin.json, so an unbumped
# change makes `claude plugin update` report "already at the latest version" and install
# nothing. .githooks/pre-commit bumps automatically — this catches the clone that never
# ran `git config core.hooksPath .githooks`, which is the whole reason CI exists here.
#
# Advisory in CI: a red run here notifies nobody. Where this actually gets read
#   is .githooks/pre-push, before the push. The message says what to do because
#   of that.
git_ok() { git -c safe.directory='*' "$@"; }
if git_ok rev-parse --git-dir >/dev/null 2>&1 && git_ok rev-parse HEAD~1 >/dev/null 2>&1; then
    changed=$(git_ok diff --name-only HEAD~1 HEAD)
    # The root .claude-plugin/marketplace.json describes plugins; it is not one, and
    #    editing a description must not demand a bump.
    for name in $(printf '%s\n' "$changed" | sed -n 's|^plugins/\([^/]*\)/.*|\1|p' | sort -u); do
        manifest="plugins/$name/.claude-plugin/plugin.json"
        printf '%s\n' "$changed" | grep -qx "$manifest" && continue
        cat >&2 <<MSG
plugins/$name/ changed but $manifest did not.
Every machine's plugin cache is keyed by that version, so this change reaches nobody.
Fix: bump the patch version in $manifest, then
  git add $manifest && git commit --amend --no-edit
and install the hook that does this for you, once per clone:
  git config core.hooksPath .githooks
MSG
        exit 1
    done
else
    echo "no parent commit reachable (shallow or root); skipping the plugin version check"
fi

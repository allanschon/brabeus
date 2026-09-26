#!/usr/bin/env bash
# The one way to run the tests BY HAND and from .githooks/pre-push, so the hook can
# never disagree with what a person runs.
#
# The gates themselves are in ci.sh, called from here and from CI. This file owns
#    only the container and the caches - CI already runs inside golang:1.25 and must
#    not pay for a second docker run.
#
# Runs in a container: no host needs a Go toolchain to keep patched.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")"
exec docker run --rm \
  -v "$PWD":/w -v /tmp/gocache:/gomod -w /w \
  -e GOMODCACHE=/gomod -e GOCACHE=/gomod/build \
  golang:1.25 ./ci.sh "$@"

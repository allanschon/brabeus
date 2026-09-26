#!/usr/bin/env bash
# Fails if any tracked file matches a pattern the deployment considers private.
# Patterns come from $LEAK_PATTERNS (one extended regex per line) and are never
# stored in this repository. With no patterns set, this check passes and says so.
set -euo pipefail
if [ -z "${LEAK_PATTERNS:-}" ]; then echo "leakcheck: no patterns set, skipping"; exit 0; fi
pat=$(printf '%s\n' "$LEAK_PATTERNS" | grep -v '^\s*$' | paste -sd'|')
if hits=$(git ls-files | xargs grep -nEi -- "$pat" 2>/dev/null); then printf '%s\n' "$hits"; echo "leakcheck: FAILED"; exit 1; fi
echo "leakcheck: clean"

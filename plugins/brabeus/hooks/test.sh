#!/usr/bin/env bash
# The plugin's hook tests. Needs bash, jq, curl, python3 — the tools the hooks
# themselves need, so a machine that can run this can run the plugin.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")"
bash brabeus-write-guard.test.sh
bash brabeus-session-start.test.sh
bash brabeus-subagent-start.test.sh
python3 brabeus-outbox.test.py
python3 brabeus-instructions.test.py

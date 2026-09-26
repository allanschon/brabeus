#!/usr/bin/env bash
# SessionStart hook for the brabeus record.
#
# Two jobs:
#   1. drain ~/.claude/memory-outbox/ into the store — anything written locally
#      while the server was unreachable;
#   2. state the routing rule, so the right path is taken first and the write
#      guard's denial is a backstop rather than the normal experience.
#
# BEST EFFORT, ALWAYS. A session must start whether or not the store is up,
# so the drain is bounded by a timeout and every failure is non-fatal. The
# outbox keeps whatever it could not send.
set -uo pipefail

HOOK_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUTBOX="$HOME/.claude/memory-outbox"
mkdir -p "$OUTBOX" 2>/dev/null || true

drained=""
if [ -n "$(find "$OUTBOX" -maxdepth 1 -name '*.md' -print -quit 2>/dev/null)" ]; then
  # Bounded: a hung server must not hold up the session.
  if out=$(timeout 20 python3 "$HOOK_DIR/brabeus-outbox-drain.py" "$OUTBOX" 2>/dev/null); then
    drained="Outbox drain: $out"
  else
    drained="Outbox drain FAILED — memories are still queued in $OUTBOX and will be retried next session."
  fi
fi

read -r -d '' CONTEXT <<CTX || true
## Memory routing

Durable facts go to the **brabeus** MCP server, not to a file. Its \`write\`
tool composes the frontmatter, updates the index and pushes to a private
repository, so the record is available on every machine.

  write(path, name, description, type, scope, body)
  scope: global | project/<owner>--<repo> | machine/<host>
  path:  <module>/<kind>/<slug>.md

Use \`list\` and \`search\` before assuming something is not already recorded.

Writing into ~/.claude/projects/*/memory/ is **denied** — that tier is per-machine
and is what this design replaced. If the server is unreachable, write a file with
\`path\`/\`name\`/\`description\`/\`scope\` frontmatter to ~/.claude/memory-outbox/ and
the next session will push it.

${drained}
CTX

jq -cn --arg c "$CONTEXT" \
  '{hookSpecificOutput:{hookEventName:"SessionStart",additionalContext:$c}}'

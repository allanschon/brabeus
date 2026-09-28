#!/usr/bin/env bash
# SessionStart hook for the brabeus record.
#
# Three jobs, in order:
#   1. drain ~/.claude/memory-outbox/ into the store — anything written locally
#      while the server was unreachable — so a drained note is in the block;
#   2. fetch the kernel's profiles from /healthz and write them where the write
#      guard reads them, so the guard's decision this session matches the
#      kernel's modules;
#   3. fetch /context and emit it as the session's first context, with a
#      three-sentence routing reminder under it.
#
# BEST EFFORT, ALWAYS. A session must start whether or not the kernel is up,
# so every step is bounded by a timeout and every failure is non-fatal. The
# outbox keeps whatever it could not send; the guard allows scratch when there
# is nothing to read.
set -uo pipefail

HOOK_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUTBOX="$HOME/.claude/memory-outbox"
mkdir -p "$OUTBOX" 2>/dev/null || true

# Read the hook's stdin ONCE, before the drain block below (which does not read
# stdin, but must not be given the chance to). Its session_id keys this
# session's profiles file in step 3. Empty stdin (`printf '' | ... `) is fine:
# jq prints nothing and $sid stays empty.
sid=$(jq -r '.session_id // empty' 2>/dev/null || true)

drained=""
if [ -n "$(find "$OUTBOX" -maxdepth 1 -name '*.md' -print -quit 2>/dev/null)" ]; then
  # Bounded: a hung server must not hold up the session.
  if out=$(timeout 20 python3 "$HOOK_DIR/brabeus-outbox-drain.py" "$OUTBOX" 2>/dev/null); then
    drained="Outbox drain: $out"
  else
    drained="Outbox drain FAILED — memories are still queued in $OUTBOX and will be retried next session."
  fi
fi

# 3. the kernel's profiles, for the guard, and the block, for the session.
#    Bounded and best effort like the drain: a session starts either way.
RUNTIME="${XDG_RUNTIME_DIR:-/tmp/brabeus-$(id -u)}/brabeus"
mkdir -p "$RUNTIME" 2>/dev/null || true
find "$RUNTIME" -maxdepth 1 -name 'profiles-*' -mtime +7 -delete 2>/dev/null || true
# sid was read above, before the drain block, so it keys this session's
# profiles file: one session starting offline cannot switch off another's
# guard. The shared file is for the /health probe, which has no session id.
write_profiles() { # $1 = one profile per line
  printf '%s\n' "$1" > "$RUNTIME/profiles" 2>/dev/null || true
  [ -n "$sid" ] && printf '%s\n' "$1" > "$RUNTIME/profiles-$sid" 2>/dev/null || true
}
drop_profiles() {
  # Build the file list as an array (non-empty, so plain expansion is safe) rather than
  # splicing ${sid:+...} unquoted into the command line, which would let a session id
  # containing whitespace word-split into more than one path.
  files=("$RUNTIME/profiles")
  [ -n "$sid" ] && files+=("$RUNTIME/profiles-$sid")
  rm -f "${files[@]}" 2>/dev/null || true
}
auth=()
[ -n "${BRABEUS_TOKEN:-}" ] && auth=(-H "Authorization: Bearer $BRABEUS_TOKEN")
# ${auth[@]+"${auth[@]}"}, not "${auth[@]}": with auth=() and `set -u`, a bare
# "${auth[@]}" is an unbound-variable error on bash < 4.4 and would abort the
# hook before it prints JSON — in the no-token case, which is the normal one.
# The +"..." form expands to nothing when the array is empty, and to the
# quoted elements otherwise, on every bash.

block=""
kernel_note=""
if [ -n "${BRABEUS_URL:-}" ] && health=$(curl -sf --max-time 5 ${auth[@]+"${auth[@]}"} "$BRABEUS_URL/healthz" 2>/dev/null); then
  # "ok <ver> identity=<mode> modules=<a,b> profiles=<p,q> claims=<when>" → one profile per line.
  write_profiles "$(printf '%s\n' "$health" | sed -n 's/.*profiles=\([^ ]*\).*/\1/p' | tr ',' '\n' | sed '/^$/d')"
  if ! block=$(curl -sf --max-time 5 ${auth[@]+"${auth[@]}"} "$BRABEUS_URL/context" 2>/dev/null); then
    block=""
    kernel_note="Kernel answered /healthz but not /context; no context block this session."
  fi
else
  # Unreachable: no profiles file, so the guard allows scratch rather than
  # stranding the session with neither store nor scratch.
  drop_profiles
  kernel_note="Kernel unreachable at ${BRABEUS_URL:-<BRABEUS_URL unset>}; no context block this session and the routing guard is off until it answers."
fi

read -r -d '' ROUTING <<CTX || true
Durable facts go to the brabeus MCP server's \`write\` tool (module, kind, scope, path), never to a file under ~/.claude/projects/*/memory/. \`context\` is the block above; \`review\` answers its agenda line. If the server is unreachable, queue a frontmattered file in ~/.claude/memory-outbox/ and the next session drains it.
CTX

CONTEXT="$block"
[ -n "$CONTEXT" ] && CONTEXT="$CONTEXT
"
CONTEXT="${CONTEXT}${ROUTING}"
[ -n "$kernel_note" ] && CONTEXT="$CONTEXT

$kernel_note"
[ -n "$drained" ] && CONTEXT="$CONTEXT

$drained"

jq -cn --arg c "$CONTEXT" \
  '{hookSpecificOutput:{hookEventName:"SessionStart",additionalContext:$c}}'

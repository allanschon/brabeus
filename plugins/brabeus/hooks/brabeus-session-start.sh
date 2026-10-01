#!/usr/bin/env bash
# SessionStart hook for the brabeus record.
#
# Three jobs, in order:
#   1. drain ~/.claude/memory-outbox/ into the store — anything written locally
#      while the server was unreachable — so a drained note is in the block;
#   2. fetch the kernel's profiles from /healthz and write them where the write
#      guard reads them, so the guard's decision this session matches the
#      kernel's modules;
#   3. fetch /context, scoped to this session's project when the cwd names
#      one, and emit it as the session's first context, with a routing
#      reminder under it that states the session's own scope keys rather than
#      leaving the model to guess them.
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

# The session's own scope keys (spec §4.2, §5), resolved here so the routing
# text below states them rather than asking the model to guess a slug.
#
# machine/<host>: the local hostname, lowercased and trimmed the way the
# kernel's scope package normalises every machine name it compares.
MACHINE=$(hostname 2>/dev/null | tr '[:upper:]' '[:lower:]' | tr -d '[:space:]')

# project/<owner>--<repo>: computed from the cwd's git remote, ssh or https.
# Outside a repository, or with no origin remote, PROJECT stays empty and no
# project scope is sent — spec §5 has no "current project" without one.
project_from_remote() { # $1 = a git remote URL (ssh://, https:// or scp-like)
  local url="${1%.git}"
  url="${url%/}"
  case "$url" in
    ssh://*|https://*|http://*)
      url="${url#*://}"   # scheme://host/... -> host/...
      url="${url#*@}"     # host may carry user@ -> host/...
      url="${url#*/}"     # host/owner/repo -> owner/repo
      ;;
    *@*:*)
      url="${url#*@}"     # user@host:owner/repo -> host:owner/repo
      url="${url#*:}"     # -> owner/repo
      ;;
    *)
      return 1
      ;;
  esac
  case "$url" in
    */*) ;;
    *) return 1 ;;
  esac
  local owner="${url%/*}" repo="${url##*/}"
  [ -n "$owner" ] && [ -n "$repo" ] && printf '%s--%s\n' "$owner" "$repo"
}
PROJECT=""
if origin=$(git -C "$PWD" remote get-url origin 2>/dev/null) && [ -n "$origin" ]; then
  PROJECT=$(project_from_remote "$origin" | tr '[:upper:]' '[:lower:]')
fi

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
# An eval case names its kernel with EVAL_BRABEUS_URL and EVAL_BRABEUS_TOKEN,
# the only variables its environment passes through (evals/README.md), so the
# session it tests starts with the block a real one would. Production never
# sets them. This comes after the outbox drain on purpose: the drain reads the
# machine's real outbox, and must never empty it into a throwaway eval kernel.
BRABEUS_URL="${BRABEUS_URL:-${EVAL_BRABEUS_URL:-}}"
BRABEUS_TOKEN="${BRABEUS_TOKEN:-${EVAL_BRABEUS_TOKEN:-}}"
auth=()
[ -n "${BRABEUS_TOKEN:-}" ] && auth=(-H "Authorization: Bearer $BRABEUS_TOKEN")
# ${auth[@]+"${auth[@]}"}, not "${auth[@]}": with auth=() and `set -u`, a bare
# "${auth[@]}" is an unbound-variable error on bash < 4.4 and would abort the
# hook before it prints JSON — in the no-token case, which is the normal one.
# The +"..." form expands to nothing when the array is empty, and to the
# quoted elements otherwise, on every bash.

CONTEXT_URL="${BRABEUS_URL:-}/context"
[ -n "$PROJECT" ] && CONTEXT_URL="${CONTEXT_URL}?project=$PROJECT"

block=""
kernel_note=""
if [ -n "${BRABEUS_URL:-}" ] && health=$(curl -sf --max-time 5 ${auth[@]+"${auth[@]}"} "$BRABEUS_URL/healthz" 2>/dev/null); then
  # "ok <ver> identity=<mode> modules=<a,b> profiles=<p,q> claims=<when>" → one profile per line.
  write_profiles "$(printf '%s\n' "$health" | sed -n 's/.*profiles=\([^ ]*\).*/\1/p' | tr ',' '\n' | sed '/^$/d')"
  if ! block=$(curl -sf --max-time 5 ${auth[@]+"${auth[@]}"} "$CONTEXT_URL" 2>/dev/null); then
    block=""
    kernel_note="Kernel answered /healthz but not /context; no context block this session."
  fi
else
  # Unreachable: no profiles file, so the guard allows scratch rather than
  # stranding the session with neither store nor scratch.
  drop_profiles
  kernel_note="Kernel unreachable at ${BRABEUS_URL:-<BRABEUS_URL unset>}; no context block this session and the routing guard is off until it answers."
fi

if [ -n "$PROJECT" ]; then
  SCOPE_KEYS="This session's own scope keys are machine/$MACHINE and project/$PROJECT; write with one of these rather than a guessed slug."
else
  SCOPE_KEYS="This session's own scope key is machine/$MACHINE; the cwd names no project (not a git repository, or no origin remote)."
fi

read -r -d '' ROUTING <<CTX || true
Durable facts go to the brabeus MCP server's \`write\` tool (module, kind, scope, path), never to a file under ~/.claude/projects/*/memory/. $SCOPE_KEYS \`context\` is the block above, and its first line is what the person's record is asking. Unless it says nothing is due, raise that question with the person once, briefly: early in your first reply, or at the first natural break if they opened with a task. Do not raise it again this session once they have answered it, put it off or passed over it. An answer to that first line given outside \`/interview\` is recorded with \`review\`, whose \`question\` is the first line's question exactly as the block gives it, with nothing added; this rule is for the first line only, and \`/interview\` sets its own for drafts. \`/interview\` works through the rest of the agenda, confirming with \`review\` and recording a manual claim's answer with \`claim_result\`. If the server is unreachable, queue a frontmattered file in ~/.claude/memory-outbox/ and the next session drains it.
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

#!/usr/bin/env bash
# PreToolUse guard, two arms.
#   1. UNCONDITIONAL: the saved copies of the person's instructions
#      (${CLAUDE_PLUGIN_DATA:-~/.claude/plugins/data/brabeus}/instructions/) are written only by
#      the session-start hook. The guard refuses the assistant's Write/Edit/NotebookEdit and its
#      common Bash write forms, which stops an accidental write of text the person never
#      confirmed; it is pattern-based, so it is not a barrier against a deliberate bypass.
#   2. CONDITIONAL on a working-memory module: memory belongs in the store, not in per-machine
#      scratch. The rest of this header describes that arm.
#
# WHY THIS EXISTS. Claude keeps an auto-memory per machine at
# ~/.claude/projects/<escaped-path>/memory/. That tier is per-machine by construction, so the
# same project ends up with different memories on different hosts and a session on one begins
# knowing nothing the other learned. The fix is a shared private store behind an MCP server.
#
# THE ROUTING IS ENFORCED, NOT DESCRIBED. The design axiom: "You should not have to remember
#    anything. If the design requires that, the design is wrong." A CLAUDE.md line asking for the
#    store is advisory, and the measured result of advisory was 73 files accumulating in scratch
#    over nine days with none promoted. So the wrong path is made UNAVAILABLE. CONDITIONAL SINCE
#    M1: enforced only where a working-memory module is enabled, decided per session from the
#    kernel's profiles.
#
# BOTH ARMS ARE REQUIRED. Measured across 30 transcripts on 2026-09-06: Bash wrote memory
#    MORE often than Write did — 20 calls against 18 — because the MEMORY.md index is appended
#    with `cat >>`. A Write-only guard would have caught the minority of operations.
#
# THE BASH ARM MATCHES WRITES, NOT MENTIONS. Reading, grepping, listing, counting and deleting
#   are all allowed. Two guards in this tree have already had to be narrowed from co-occurrence
#   to actual use; this one starts there. Counting scratch files is how the backstop check
#   detects a machine where this guard is missing — blocking that would disable its own detector.
#
# ESCAPE HATCH, deliberately visible in the transcript: MEMORYGUARD_OVERRIDE=<reason> in a Bash
# command. There is none for Write/Edit: those have nowhere to carry a reason, and the whole
# point is that the file path is not a decision anyone should be making.
#
# Exit 0 with no output = allow.
set -uo pipefail

payload=$(cat)
tool=$(printf '%s' "$payload" | jq -r '.tool_name // empty' 2>/dev/null)

deny() {
  jq -cn --arg r "$1" \
    '{hookSpecificOutput:{hookEventName:"PreToolUse",permissionDecision:"deny",permissionDecisionReason:$r}}'
  exit 0
}

# The Write/Edit path, made absolute with . and .. collapsed textually: the file need not
# exist, so realpath is not an option, and a path check that can be walked around with .. is
# not a check. Prints nothing when the call names no path.
norm_path() {
  local fp
  fp=$(printf '%s' "$payload" | jq -r '.tool_input.file_path // .tool_input.notebook_path // empty' 2>/dev/null)
  [ -z "$fp" ] && return 0
  case "$fp" in
    "~"*) fp="$HOME${fp#\~}" ;;
    /*)   ;;
    *)    fp="$PWD/$fp" ;;
  esac
  printf '%s' "$fp" | awk -F/ '{n=0; for(i=1;i<=NF;i++){ if($i=="."||$i==""){continue} if($i==".."){if(n>0)n--; continue} p[++n]=$i } s=""; for(i=1;i<=n;i++) s=s"/"p[i]; print (s==""?"/":s)}'
}

# A heredoc BODY is data the command writes, not commands it runs, and it is dropped. The
# heredoc HEADER survives, so `cat > <path> <<EOF` is still seen. Then one line, so a body
# cannot hide the redirect that opened it. Sets $flat.
flatten_cmd() {
  local c
  c=$(printf '%s\n' "$1" | awk '
    !inh {
      print
      if (match($0, /<<-?[ \t]*[\047\042]?[A-Za-z_][A-Za-z0-9_]*[\047\042]?/)) {
        tag = substr($0, RSTART, RLENGTH)
        sub(/^<<-?[ \t]*/, "", tag); gsub(/[\047\042]/, "", tag)
        inh = 1
      }
      next
    }
    { t = $0; sub(/^[ \t]+/, "", t); sub(/[ \t]+$/, "", t); if (t == tag) inh = 0 }
  ')
  flat=$(printf '%s' "$c" | tr '\n' ' ')
}

# Does the flattened command WRITE to a path matching regex $1? Each pattern names a way of
# writing. Reads are absent on purpose.
writes_to() {
  local P="$1"
  rx() { printf '%s' "$flat" | grep -qE "$1"; }
  rx ">>?[[:space:]]*${P}" \
  || rx "tee([[:space:]]+-[^[:space:]]+)*[[:space:]]+${P}" \
  || rx "sed[^|;&]*-i[^|;&]*${P}" \
  || rx "(cp|mv|install|touch|ln)[[:space:]][^|;&]*${P}" \
  || rx "dd[^|;&]*of=${P}"
}

# ARM 1, UNCONDITIONAL. The saved copies of the person's instructions are written only by the
# session-start hook. Text the person never confirmed must not reach a subagent as their
# instructions, so no profile, no override and no unreachable kernel lifts this.
DATA_DIR="${CLAUDE_PLUGIN_DATA:-$HOME/.claude/plugins/data/brabeus}"
COPIES="${DATA_DIR%/}/instructions/"
COPIES_REASON='The saved instructions are written only by the plugin'"'"'s session-start hook, so text the person never confirmed cannot reach a subagent as their instructions. Change an instruction through the interview.'
case "$tool" in
Write|Edit|NotebookEdit)
  fp=$(norm_path)
  case "$fp" in
    "$COPIES"*) deny "$COPIES_REASON" ;;
  esac
  ;;
Bash)
  cmd=$(printf '%s' "$payload" | jq -r '.tool_input.command // empty' 2>/dev/null)
  if [ -n "$cmd" ]; then
    flatten_cmd "$cmd"
    esc=$(printf '%s' "$COPIES" | sed 's/[][\.*^$+?(){}|\/]/\\&/g')
    P="[^ ]*${esc}"
    case "$COPIES" in
      "$HOME"/*) P="[^ ]*(${esc}|(~|\\\$HOME|\\\$\\{HOME\\})$(printf '%s' "${COPIES#"$HOME"}" | sed 's/[][\.*^$+?(){}|\/]/\\&/g'))" ;;
    esac
    writes_to "$P" && deny "$COPIES_REASON"
  fi
  ;;
esac

# ARM 2, CONDITIONAL SINCE M1. hooks.json is static, so the guard decides here. The
# session-start hook writes the kernel's profiles to this file; scratch is denied only where a
# working-memory module is enabled. No file means the kernel was unreachable, and a
# session must not lose its store and its scratch at once: allow.
RUNTIME="${XDG_RUNTIME_DIR:-/tmp/brabeus-$(id -u)}/brabeus"
sid=$(printf '%s' "$payload" | jq -r '.session_id // empty' 2>/dev/null)
PROFILES="$RUNTIME/profiles"
[ -n "$sid" ] && [ -f "$RUNTIME/profiles-$sid" ] && PROFILES="$RUNTIME/profiles-$sid"
if ! grep -qx 'working-memory' "$PROFILES" 2>/dev/null; then
  exit 0
fi

REASON='Memory belongs in the shared store, not in this machine'"'"'s scratch directory.

Use the brabeus MCP server instead:
  write(path, name, description, module, kind, scope, body)

It composes the frontmatter, updates MEMORY.md and pushes to the record'"'"'s repository in one
call, so the memory is available on every machine rather than only this one.

scope: global | project/<owner>--<repo> | machine/<host>

If the server is unreachable, write to ~/.claude/memory-outbox/ instead; the next session drains
it. Design: docs/personal-context-system-v1.md'

case "$tool" in
Write|Edit|NotebookEdit)
  fp=$(norm_path)
  [ -z "$fp" ] && exit 0
  case "$fp" in
    "$HOME"/.claude/projects/*/memory/*) deny "$REASON" ;;
  esac
  exit 0
  ;;
Bash)
  cmd=$(printf '%s' "$payload" | jq -r '.tool_input.command // empty' 2>/dev/null)
  [ -z "$cmd" ] && exit 0
  case "$cmd" in *MEMORYGUARD_OVERRIDE=*) exit 0 ;; esac

  # NARROWED 2026-09-07: the heredoc body is dropped before matching (see flatten_cmd).
  #    Measured false positive: writing a plan document that QUOTED the scratch path was denied,
  #    because flattening makes `[^|;&]*` span the whole document. Proven by three cases in the test.
  flatten_cmd "$cmd"
  writes_to '[^ ]*\.claude/projects/[^ ]*/memory/' && deny "$REASON"
  exit 0
  ;;
esac
exit 0

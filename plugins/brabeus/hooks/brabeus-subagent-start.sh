#!/usr/bin/env bash
# SubagentStart hook: give every subagent the person's instructions (spec §4.2,
# §10). A subagent starts with neither the main session's conversation nor the
# block, so a rule only those carry is broken by the agent doing the work. It
# reads the copy this session's SessionStart saved and makes no network call.
set -uo pipefail
HOOK_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
sid=$(jq -r '.session_id // empty' 2>/dev/null || true)
[ -z "$sid" ] && exit 0
DATA="${CLAUDE_PLUGIN_DATA:-$HOME/.claude/plugins/data/brabeus}/instructions"
text=$(python3 "$HOOK_DIR/brabeus-instructions.py" text "$DATA" "$sid" 9800 2>/dev/null) || exit 0
[ -z "$text" ] && exit 0
jq -cn --arg c "$text" '{hookSpecificOutput:{hookEventName:"SubagentStart",additionalContext:$c}}'

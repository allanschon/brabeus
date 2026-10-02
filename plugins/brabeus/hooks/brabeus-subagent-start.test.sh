#!/usr/bin/env bash
# Regression test for brabeus-subagent-start.sh: a subagent gets the person's
# instructions from the copy its session saved, says so when there is no copy,
# and says nothing when the person has no instructions or the input has no session.
set -uo pipefail
HOOK="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/brabeus-subagent-start.sh"
export HOME="$(mktemp -d)"
unset CLAUDE_PLUGIN_DATA                   # copies are read from the default path under HOME
trap 'rm -rf "$HOME"' EXIT
DATA="$HOME/.claude/plugins/data/brabeus/instructions"
mkdir -p "$DATA"
printf '%s' '{"opening":"OPEN","records":[{"module":"identity","path":"identity/preference/a.md","text":"Rule A."}],"sizes":[]}' > "$DATA/s1.json"
printf '%s' '{"opening":"OPEN","records":[],"sizes":[]}' > "$DATA/s3.json"

pass=0; fail=0
check() { if eval "$2"; then pass=$((pass+1)); printf 'ok   %s\n' "$1"; else fail=$((fail+1)); printf 'FAIL %s\n' "$1"; fi; }
run() { printf '%s' "$1" | bash "$HOOK"; }

out=$(run '{"session_id":"s1","hook_event_name":"SubagentStart"}'); rc=$?
ctx=$(printf '%s' "$out" | jq -r '.hookSpecificOutput.additionalContext')
check "exits 0 with a copy"                   '[ "$rc" = 0 ]'
check "emits SubagentStart JSON"              '[ "$(printf "%s" "$out" | jq -r .hookSpecificOutput.hookEventName)" = SubagentStart ]'
check "carries the opening"                   'printf "%s" "$ctx" | grep -q "^OPEN"'
check "carries the record"                    'printf "%s" "$ctx" | grep -q "Rule A\."'

out=$(run '{"session_id":"s3"}'); rc=$?
check "a copy with no records says nothing"   '[ -z "$out" ] && [ "$rc" = 0 ]'

out=$(run '{"session_id":"s2"}')
check "no copy says the instructions are unavailable" '[ "$(printf "%s" "$out" | jq -r .hookSpecificOutput.additionalContext)" = "The person'"'"'s standing instructions are unavailable to this agent: no saved copy for this session." ]'

out=$(run '{"hook_event_name":"SubagentStart"}'); rc=$?
check "no session_id says nothing"            '[ -z "$out" ] && [ "$rc" = 0 ]'

out=$(run '{"session_id":"../x"}'); rc=$?
check "an unsafe session_id says nothing"     '[ -z "$out" ] && [ "$rc" = 0 ]'

out=$(CLAUDE_PLUGIN_DATA="$HOME/other" run '{"session_id":"s1"}')
check "CLAUDE_PLUGIN_DATA moves the copies"   'printf "%s" "$out" | jq -r .hookSpecificOutput.additionalContext | grep -q "unavailable to this agent"'

echo
echo "$((pass+fail)) cases · $pass ok · $fail failed"
[ "$fail" -gt 0 ] && exit 1
exit 0

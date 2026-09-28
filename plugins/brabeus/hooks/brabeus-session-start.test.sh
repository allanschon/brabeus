#!/usr/bin/env bash
# Regression test for brabeus-session-start.sh: the block is the session's first
# context when the kernel answers; the session still starts when it does not.
# A stub kernel on a loopback port stands in for the real one.
set -uo pipefail
HOOK="$(dirname "${BASH_SOURCE[0]}")/brabeus-session-start.sh"
export HOME="$(mktemp -d)"                 # an empty outbox, nothing drained
export XDG_RUNTIME_DIR="$(mktemp -d)"
trap 'rm -rf "$HOME" "$XDG_RUNTIME_DIR"; kill "$srv" 2>/dev/null' EXIT
PROFILES="$XDG_RUNTIME_DIR/brabeus/profiles"

port=$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1])')
python3 - "$port" <<'PY' &
import sys
from http.server import BaseHTTPRequestHandler, HTTPServer
class H(BaseHTTPRequestHandler):
    def log_message(self, *a): pass
    def do_GET(self):
        if self.path == "/healthz":
            body = b"ok 0.3.0 identity=fake modules=memory,identity profiles=working-memory,ratified-record claims=2026-09-20T04:00:00Z\n"
        elif self.path == "/context":
            if self.headers.get("Authorization") != "Bearer " + "t" * 12:
                self.send_response(401); self.end_headers(); return
            body = b"agenda: [identity/value family] Still one of the things you weigh decisions against?\nidentity:\n- value: family first\n"
        else:
            self.send_response(404); self.end_headers(); return
        self.send_response(200); self.send_header("Content-Type", "text/plain; charset=utf-8"); self.end_headers(); self.wfile.write(body)
HTTPServer(("127.0.0.1", int(sys.argv[1])), H).serve_forever()
PY
srv=$!
for _ in $(seq 40); do curl -s "http://127.0.0.1:$port/healthz" >/dev/null 2>&1 && break; sleep 0.1; done

pass=0; fail=0
check() { if eval "$2"; then pass=$((pass+1)); printf 'ok   %s\n' "$1"; else fail=$((fail+1)); printf 'FAIL %s\n' "$1"; fi; }

# ── reachable kernel ─────────────────────────────────────────────────────────────────────────
out=$(printf '{"session_id":"s1","hook_event_name":"SessionStart"}' | BRABEUS_URL="http://127.0.0.1:$port" BRABEUS_TOKEN="$(printf 't%.0s' $(seq 12))" bash "$HOOK")
ctx=$(printf '%s' "$out" | jq -r '.hookSpecificOutput.additionalContext')
check "emits valid SessionStart JSON"         '[ "$(printf "%s" "$out" | jq -r .hookSpecificOutput.hookEventName)" = SessionStart ]'
check "the block is the first context"       '[ "$(printf "%s" "$ctx" | head -1)" = "agenda: [identity/value family] Still one of the things you weigh decisions against?" ]'
check "the routing reminder follows"         'printf "%s" "$ctx" | grep -q "brabeus.*write"'
check "profiles read up to claims= after them" '[ "$(cat "$PROFILES")" = "$(printf "working-memory\nratified-record")" ]'
check "per-session profiles file written"    '[ "$(cat "$PROFILES-s1")" = "$(cat "$PROFILES")" ]'

# ── reachable kernel, wrong token: /healthz answers, /context 401s ─────────────────────────────
out=$(printf '{"session_id":"s1"}' | BRABEUS_URL="http://127.0.0.1:$port" BRABEUS_TOKEN="$(printf 'x%.0s' $(seq 5))" bash "$HOOK")
ctx=$(printf '%s' "$out" | jq -r '.hookSpecificOutput.additionalContext')
check "healthz-ok/context-fails: valid JSON"       '[ "$(printf "%s" "$out" | jq -r .hookSpecificOutput.hookEventName)" = SessionStart ]'
check "healthz-ok/context-fails: says so"          'printf "%s" "$ctx" | grep -q "answered /healthz but not /context"'
check "healthz-ok/context-fails: profiles written" '[ "$(cat "$PROFILES")" = "$(printf "working-memory\nratified-record")" ]'
check "healthz-ok/context-fails: routing present"  'printf "%s" "$ctx" | grep -q "brabeus.*write"'

# ── unreachable kernel ───────────────────────────────────────────────────────────────────────
out=$(printf '{"session_id":"s1"}' | BRABEUS_URL="http://127.0.0.1:1" bash "$HOOK")
ctx=$(printf '%s' "$out" | jq -r '.hookSpecificOutput.additionalContext')
check "still emits valid JSON when down"     '[ "$(printf "%s" "$out" | jq -r .hookSpecificOutput.hookEventName)" = SessionStart ]'
check "says the kernel is unreachable"       'printf "%s" "$ctx" | grep -qi "unreachable"'
check "routing text still present when down" 'printf "%s" "$ctx" | grep -q "brabeus.*write"'
check "profiles files removed when down"     '[ ! -e "$PROFILES" ] && [ ! -e "$PROFILES-s1" ]'

# ── no URL configured ────────────────────────────────────────────────────────────────────────
out=$(printf '' | env -u BRABEUS_URL bash "$HOOK")
check "no BRABEUS_URL: valid JSON, no crash" '[ "$(printf "%s" "$out" | jq -r .hookSpecificOutput.hookEventName)" = SessionStart ]'

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]

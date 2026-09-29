#!/usr/bin/env bash
# Regression test for brabeus-session-start.sh: the block is the session's first
# context when the kernel answers; the session still starts when it does not;
# the project scope key is computed from the cwd's git remote and sent to /context.
# A stub kernel on a loopback port stands in for the real one.
set -uo pipefail
HOOK="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/brabeus-session-start.sh"
export HOME="$(mktemp -d)"                 # an empty outbox, nothing drained
export XDG_RUNTIME_DIR="$(mktemp -d)"
# WORKDIR is not, and is not inside, a git repository: every test below that
# does not set up its own repo runs from here, so PROJECT stays empty
# regardless of where this test happens to be checked out.
WORKDIR="$(mktemp -d)"
REQLOG="$(mktemp)"
trap 'rm -rf "$HOME" "$XDG_RUNTIME_DIR" "$WORKDIR" "$REQLOG" "${REPO:-}" "${REPO2:-}"; kill "$srv" 2>/dev/null' EXIT
PROFILES="$XDG_RUNTIME_DIR/brabeus/profiles"

port=$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1])')
python3 - "$port" "$REQLOG" <<'PY' &
import sys
from http.server import BaseHTTPRequestHandler, HTTPServer
class H(BaseHTTPRequestHandler):
    def log_message(self, *a): pass
    def do_GET(self):
        with open(sys.argv[2], "a") as f:
            f.write(self.path + "\n")
        base = self.path.split("?", 1)[0]
        if base == "/healthz":
            # errors=2 after claims= is real M2 output (a goal whose results
            # could not be recorded); the fixture carries it so this test
            # keeps proving the profiles= regex stops at the next space
            # regardless of what follows claims=.
            body = b"ok 0.3.0 identity=fake modules=memory,identity profiles=working-memory,ratified-record claims=2026-09-20T04:00:00Z errors=2\n"
        elif base == "/context":
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

# ── reachable kernel, no project (cwd is not a git repository) ─────────────────────────────────
out=$(cd "$WORKDIR" && printf '{"session_id":"s1","hook_event_name":"SessionStart"}' | BRABEUS_URL="http://127.0.0.1:$port" BRABEUS_TOKEN="$(printf 't%.0s' $(seq 12))" bash "$HOOK")
ctx=$(printf '%s' "$out" | jq -r '.hookSpecificOutput.additionalContext')
check "emits valid SessionStart JSON"         '[ "$(printf "%s" "$out" | jq -r .hookSpecificOutput.hookEventName)" = SessionStart ]'
check "the block is the first context"       '[ "$(printf "%s" "$ctx" | head -1)" = "agenda: [identity/value family] Still one of the things you weigh decisions against?" ]'
check "the routing reminder follows"         'printf "%s" "$ctx" | grep -q "brabeus.*write"'
check "the routing names the interview"      'printf "%s" "$ctx" | grep -q "/interview.*claim_result"'
check "profiles read up to claims=, ignoring errors= after it" '[ "$(cat "$PROFILES")" = "$(printf "working-memory\nratified-record")" ]'
check "per-session profiles file written"    '[ "$(cat "$PROFILES-s1")" = "$(cat "$PROFILES")" ]'
check "no project outside a git repository"  'printf "%s" "$ctx" | grep -q "cwd names no project"'
check "no project query sent to /context"    '! tail -1 "$REQLOG" | grep -q "project="'

# ── the project key: cwd is a git repository with an ssh origin ────────────────────────────────
REPO="$(mktemp -d)"
git -C "$REPO" init -q
git -C "$REPO" remote add origin ssh://forge.example/owner/repo.git
out=$(cd "$REPO" && printf '{"session_id":"s4"}' | BRABEUS_URL="http://127.0.0.1:$port" BRABEUS_TOKEN="$(printf 't%.0s' $(seq 12))" bash "$HOOK")
ctx=$(printf '%s' "$out" | jq -r '.hookSpecificOutput.additionalContext')
check "the project query reaches /context"    'grep -q "^/context?project=owner--repo$" "$REQLOG"'
check "the routing states this session's project key" 'printf "%s" "$ctx" | grep -q "project/owner--repo"'
check "the routing states this session's machine key"  'printf "%s" "$ctx" | grep -qE "machine/[a-z0-9._-]+"'

# ── the project key round-trips when the repository name itself has "--" ───────────────────────
# GitHub and Gitea both allow "--" in an owner or repository name, so the joined key can carry
# more than one run of it. CheckProjectKey must accept this key on the read side exactly as
# CheckScope always accepted it on the write side (internal/scope).
REPO2="$(mktemp -d)"
git -C "$REPO2" init -q
git -C "$REPO2" remote add origin ssh://forge.example/acme/my--tool.git
out=$(cd "$REPO2" && printf '{"session_id":"s5"}' | BRABEUS_URL="http://127.0.0.1:$port" BRABEUS_TOKEN="$(printf 't%.0s' $(seq 12))" bash "$HOOK")
ctx=$(printf '%s' "$out" | jq -r '.hookSpecificOutput.additionalContext')
check "a repo name containing -- still computes owner--repo" 'grep -q "^/context?project=acme--my--tool$" "$REQLOG"'
check "the routing states the double-dash project key"       'printf "%s" "$ctx" | grep -q "project/acme--my--tool"'

# ── reachable kernel, wrong token: /healthz answers, /context 401s ─────────────────────────────
out=$(cd "$WORKDIR" && printf '{"session_id":"s1"}' | BRABEUS_URL="http://127.0.0.1:$port" BRABEUS_TOKEN="$(printf 'x%.0s' $(seq 5))" bash "$HOOK")
ctx=$(printf '%s' "$out" | jq -r '.hookSpecificOutput.additionalContext')
check "healthz-ok/context-fails: valid JSON"       '[ "$(printf "%s" "$out" | jq -r .hookSpecificOutput.hookEventName)" = SessionStart ]'
check "healthz-ok/context-fails: says so"          'printf "%s" "$ctx" | grep -q "answered /healthz but not /context"'
check "healthz-ok/context-fails: profiles written" '[ "$(cat "$PROFILES")" = "$(printf "working-memory\nratified-record")" ]'
check "healthz-ok/context-fails: routing present"  'printf "%s" "$ctx" | grep -q "brabeus.*write"'

# ── unreachable kernel ───────────────────────────────────────────────────────────────────────
out=$(cd "$WORKDIR" && printf '{"session_id":"s1"}' | BRABEUS_URL="http://127.0.0.1:1" bash "$HOOK")
ctx=$(printf '%s' "$out" | jq -r '.hookSpecificOutput.additionalContext')
check "still emits valid JSON when down"     '[ "$(printf "%s" "$out" | jq -r .hookSpecificOutput.hookEventName)" = SessionStart ]'
check "says the kernel is unreachable"       'printf "%s" "$ctx" | grep -qi "unreachable"'
check "routing text still present when down" 'printf "%s" "$ctx" | grep -q "brabeus.*write"'
check "profiles files removed when down"     '[ ! -e "$PROFILES" ] && [ ! -e "$PROFILES-s1" ]'

# ── no URL configured ────────────────────────────────────────────────────────────────────────
out=$(cd "$WORKDIR" && printf '' | env -u BRABEUS_URL bash "$HOOK")
check "no BRABEUS_URL: valid JSON, no crash" '[ "$(printf "%s" "$out" | jq -r .hookSpecificOutput.hookEventName)" = SessionStart ]'

printf '\n%d passed, %d failed\n' "$pass" "$fail"
[ "$fail" -eq 0 ]

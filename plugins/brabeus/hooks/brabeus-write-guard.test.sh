#!/usr/bin/env bash
# Regression test for brabeus-write-guard.sh.
#
# WHY. The guard is the whole mechanism the memory design rests on: routing is enforced, not
# described. It denies writes GLOBALLY, on every project on every machine, so a false positive
# does not annoy — it stops the person saving anything, everywhere.
#
# Both arms matter. On 2026-09-06, 30 transcripts showed Bash writing memory MORE often than
#    Write did (20 vs 18) because the index is appended with `cat >>`. A Write-only guard would
#    have caught the minority of operations.
#
# Usage: .claude/hooks/brabeus-write-guard.test.sh [path-to-guard]
set -uo pipefail
GUARD="${1:-$(dirname "${BASH_SOURCE[0]}")/brabeus-write-guard.sh}"
unset CLAUDE_PLUGIN_DATA
C="$HOME/.claude/plugins/data/brabeus/instructions"
M="$HOME/.claude/projects/$(printf '%s' "$PWD" | sed 's|/|-|g')/memory"

# The guard decides per session from the kernel's profiles. The session-start hook
# writes this file; the tests write it themselves so the guard is tested alone.
export XDG_RUNTIME_DIR="$(mktemp -d)"
trap 'rm -rf "$XDG_RUNTIME_DIR"' EXIT
PROFILES="$XDG_RUNTIME_DIR/brabeus/profiles"
mkdir -p "$(dirname "$PROFILES")"
printf 'working-memory\nratified-record\n' > "$PROFILES"

pass=0; fail=0
check() { # $1=want  $2=name  $3=tool  $4=payload-field-value
  local want="$1" name="$2" tool="$3" val="$4" out got key=file_path
  [ "$tool" = Bash ] && key=command
  out=$(jq -cn --arg t "$tool" --arg k "$key" --arg v "$val" --arg s "${SID:-}" \
        '{tool_name:$t, tool_input:{($k):$v}} + (if $s != "" then {session_id:$s} else {} end)' | bash "$GUARD" 2>/dev/null)
  if [ -z "$out" ]; then got=allow
  else got=$(printf '%s' "$out" | jq -r '.hookSpecificOutput.permissionDecision'); fi
  if [ "$got" = "$want" ]; then pass=$((pass+1)); printf 'ok   %-46s %s\n' "$name" "$got"
  else fail=$((fail+1)); printf 'FAIL %-46s want=%s got=%s\n' "$name" "$want" "$got"; fi
}

# ── the Write/Edit arm ──────────────────────────────────────────────────────────────────────
check deny  "write a memory file"            Write "$M/some-fact.md"
check deny  "edit a memory file"             Edit  "$M/some-fact.md"
check deny  "write the scratch index"        Write "$M/MEMORY.md"
check deny  "tilde path resolves into memory" Write "~/.claude/projects/-x/memory/a.md"
check deny  "dot-dot cannot escape the match" Write "$HOME/.claude/projects/-x/other/../memory/a.md"

# ── the Bash arm — this is the majority of real memory writes ───────────────────────────────
check deny  "append to the index with cat"   Bash "cat >> $M/MEMORY.md <<'X'
- [a](a.md) - hook
X"
check deny  "tee into a memory file"         Bash "echo hi | tee $M/a.md"
check deny  "redirect into a memory file"    Bash "echo hi > $M/a.md"
check deny  "sed -i a memory file"           Bash "sed -i 's/a/b/' $M/a.md"
check deny  "cp into the memory dir"         Bash "cp /tmp/a.md $M/a.md"
check deny  "mv into the memory dir"         Bash "mv /tmp/a.md $M/"

# ── reads and housekeeping must NOT be blocked ──────────────────────────────────────────────
# A backstop check COUNTS these files. A guard that blocked counting them
#    would disable its own detector.
check allow "count scratch files"            Bash "find ~/.claude/projects -path '*/memory/*.md' | wc -l"
check allow "read a memory file"             Bash "cat $M/a.md"
check allow "grep the memory dir"            Bash "grep -r foo $M/"
check allow "list the memory dir"            Bash "ls -la $M/"
# Deletion is cleanup, not authoring. The migration needed exactly this.
check allow "delete a scratch memory"        Bash "rm $M/a.md"

# ── the outbox is the sanctioned local path when the server is unreachable ──────────────────
check allow "write to the outbox"            Write "$HOME/.claude/memory-outbox/a.md"
check allow "append to the outbox"           Bash "cat >> $HOME/.claude/memory-outbox/a.md <<'X'
x
X"

# ── everything else is none of this guard's business ────────────────────────────────────────
check allow "an ordinary project file"       Write "$HOME/Projects/example-app/README.md"
check allow "a file merely named memory.md"  Write "$HOME/Projects/memory.md"
check allow "a src file"                     Write "$HOME/src/example-app/main.py"
check allow "an unrelated command"           Bash "git -C ~/Projects status"

# ── the escape hatch, deliberately visible in the transcript ────────────────────────────────
check allow "override on a Bash write"       Bash "MEMORYGUARD_OVERRIDE=migrating cp /tmp/a.md $M/a.md"

# ── heredoc BODIES are data, not commands ───────────────────────────────────────────────────
# Measured false positive, 2026-09-07: writing a PLAN DOCUMENT that quotes the scratch path
#    was denied. The guard flattens the command to one line so a heredoc body cannot hide the
#    redirect that opened it — but `[^|;&]*` then spans the whole flattened document, so a `cp`
#    or `sed -i` anywhere matches a path mentioned anywhere later.
# Stripping the BODY keeps the HEADER, so the redirect the flattening protects is untouched.
check allow "a doc whose body quotes the path" Bash "cat > /tmp/plan.md <<'EOF'
Run: cp a b, then sed -i s/x/y/ file
The guard denies $M/probe.md and that is the point.
EOF"
check allow "a doc body mentioning tee"        Bash "cat > /tmp/notes.md <<'EOF'
Example: echo hi | tee $M/a.md
EOF"
check deny  "heredoc HEADER still targets memory" Bash "cat > $M/a.md <<'EOF'
harmless body
EOF"

# ── the guard is conditional on the kernel's profiles ───────────────────────────────────────
printf 'ratified-record\n' > "$PROFILES"
check allow "ratified-only deployment allows scratch" Write "$M/some-fact.md"
check allow "ratified-only deployment allows bash"    Bash  "echo hi > $M/a.md"
rm -f "$PROFILES"
check allow "no profiles file (kernel unreachable) allows" Write "$M/some-fact.md"
printf 'working-memory\n' > "$PROFILES"
check deny  "working-memory listed denies again"     Write "$M/some-fact.md"
# Per-session file wins over the shared one, and only for that session.
printf 'ratified-record\n' > "$XDG_RUNTIME_DIR/brabeus/profiles-s1"
SID=s1 check allow "own session file (ratified only) allows"  Write "$M/some-fact.md"
SID=s2 check deny  "another session falls back to shared"     Write "$M/some-fact.md"

# ── the saved instructions are written only by the session-start hook ───────────────────────
# Unconditional: decided without the profiles file, so it holds where no working-memory module
# is enabled and where the kernel was unreachable.
printf 'ratified-record\n' > "$PROFILES"
check deny  "write a saved instructions copy"  Write "$C/s1.json"
check deny  "edit a saved instructions copy"   Edit  "$C/s1.json"
check deny  "dot-dot cannot escape the copies" Write "$C/../instructions/s1.json"
check deny  "cat into a saved copy"            Bash "cat > $C/s1.json"
check deny  "sed -i a saved copy"              Bash "sed -i 's/a/b/' $C/s1.json"
check deny  "cp into the copies dir"           Bash "cp x $C/"
check deny  "tilde form of a copy"             Bash "echo x > ~/.claude/plugins/data/brabeus/instructions/s1.json"
check deny  "HOME form of a copy"              Bash 'echo x > $HOME/.claude/plugins/data/brabeus/instructions/s1.json'
check allow "cat a saved copy"                 Bash "cat $C/s1.json"
check allow "list the copies dir"              Bash "ls $C"
check allow "a sibling directory"              Write "$HOME/.claude/plugins/data/brabeus/other/s1.json"
CLAUDE_PLUGIN_DATA=/srv/pd check deny  "CLAUDE_PLUGIN_DATA moves the copies (Write)" Write "/srv/pd/instructions/s1.json"
CLAUDE_PLUGIN_DATA=/srv/pd check deny  "CLAUDE_PLUGIN_DATA moves the copies (Bash)"  Bash "cp x /srv/pd/instructions/"
CLAUDE_PLUGIN_DATA=/srv/pd check allow "the default path is then not a copy"        Write "$C/s1.json"
rm -f "$PROFILES"
check deny  "no profiles file still denies a copy" Write "$C/s1.json"
printf 'working-memory\n' > "$PROFILES"

echo
echo "$((pass+fail)) cases · $pass ok · $fail failed"
[ "$fail" -gt 0 ] && exit 1
exit 0

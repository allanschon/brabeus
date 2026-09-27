---
name: health
description: Use when checking whether this machine's brabeus routing is working - the guard is active, nothing is stuck in the outbox, and nothing has accumulated in per-machine scratch. Also use when setting up a new machine.
---

# Is this machine's memory routing working?

Answer for THIS machine only. There is deliberately no way to ask about another
machine: reaching across the network is what made the previous checker
unreliable — it was blind to a sleeping laptop and reported "could not reach it"
as a failure, so it was red most nights for a boring reason.

Run all five checks and give one verdict.

## 0. Did the kernel answer, and did it render the block?

```bash
curl -sf --max-time 5 ${BRABEUS_TOKEN:+-H "Authorization: Bearer $BRABEUS_TOKEN"} "$BRABEUS_URL/healthz"
```

Expect one line starting `ok`, naming the modules and profiles. Then call the
`context` tool with no arguments: the block must be under 2048 bytes and `faults`
must be absent. A fault names a module whose summary is over its budget, or
`agenda` when its line had to be cut; report it, it is a template or budget
problem, not a data problem. If the first line is `agenda: nothing due` and you
know a record is old, that is worth saying: the agenda is computed from what
this machine may see.

## 1. Is the guard actually refusing?

Presence is not evidence. Exercise it:

```bash
printf '{"tool_name":"Write","tool_input":{"file_path":"%s/.claude/projects/x/memory/probe.md"}}' "$HOME" \
  | "${CLAUDE_PLUGIN_ROOT}/hooks/brabeus-write-guard.sh"
```

Judge this on STDOUT, never on the exit code. Every path in the guard exits 0,
including the denial — a PreToolUse hook refuses by printing a decision, not by
failing. The guard is conditional since M1: it decides from the session's
profiles file at `${XDG_RUNTIME_DIR:-/tmp/brabeus-$(id -u)}/brabeus/profiles`.
`cat` that file before judging. When it lists `working-memory`, a live guard
prints `"permissionDecision":"deny"` and silence is the headline. When it lists
only `ratified-record`, or is absent because the kernel was unreachable,
silence is correct and the report says why.

## 2. Has anything accumulated in per-machine scratch?

```bash
find ~/.claude/projects -path "*/memory/*.md" 2>/dev/null | wc -l
```

Expect 0. Anything above 0 means memories have been going somewhere only this
machine can see. List them — they are worth reading before deleting, and the
`write` tool is how they get into the shared store.

## 3. Is anything stuck in the outbox?

```bash
find ~/.claude/memory-outbox -name '*.md' 2>/dev/null | wc -l
```

Expect 0. The outbox holds memories written while the server was unreachable,
and the SessionStart hook drains it. A count that persists across two sessions
means the drain is failing, not that the server was briefly down.

## 4. Are working notes dating themselves?

A working note that is neither dated, a pointer, nor timeless will be wrong one
day with no sign of it. Call `list` and look at the `memory` module's entries:

- **timeless** — the kind says so in the manifest (`preference` is);
- **dated** — the description or path carries a date (`2026-09-27`, `2026-09`);
- **a pointer** — the description names a path, a URL, a file or a host to look at.

Report the entries that are none of the three, up to twenty, path and
description, as notes worth a review — nothing more. This is a heuristic on the
description alone; it does not read bodies, and it is not a verdict. Do not
delete or rewrite anything from this check.

## Reporting

Say plainly which of the five passed. If all five pass, one line is enough.

Check 0 failing is the headline — a dark kernel makes the rest moot. Checks 2
and 3 are only meaningful once check 1 says the guard is live *and* check 0
says the kernel is up: both read zero on a machine where nothing is happening
at all, and a guard that is off because the kernel is down is expected, not a
fault. Check 4 is a list to review, not a pass/fail.

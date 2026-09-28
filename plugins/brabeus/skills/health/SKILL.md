---
name: health
description: Use when checking whether this machine's brabeus routing is working - the guard is active, nothing is stuck in or rejected from the outbox, nothing has accumulated in per-machine scratch - and whether the failures that would otherwise be silent have happened - a module over its byte budget, the claim schedule not running, an adapter unable to answer. Also use when setting up a new machine.
---

# Is this machine's memory routing working?

Answer for THIS machine only. There is deliberately no way to ask about another
machine: reaching across the network is what made the previous checker
unreliable — it was blind to a sleeping laptop and reported "could not reach it"
as a failure, so it was red most nights for a boring reason.

Run all seven checks and give one verdict. Four of them look for failures that nothing
else would show — a module over its byte budget (check 0), an outbox write the kernel
rejected (check 3), the claim schedule not running (check 4) and an adapter that cannot
answer (check 5) — because each one leaves the record quietly wrong rather than visibly
broken.

## 0. Did the kernel answer, and did it render the block?

```bash
curl -sf --max-time 5 ${BRABEUS_TOKEN:+-H "Authorization: Bearer $BRABEUS_TOKEN"} "$BRABEUS_URL/healthz"
```

Expect one line starting `ok`, naming the modules and profiles. Then call the
`context` tool with no arguments: the block must be under 2048 bytes and `faults`
must be absent. A fault names a module whose summary is over its budget, or
`agenda` when its line had to be cut. That is the budget failure: the module's
share of the block is one line saying so, and every session is missing what it
would have said. Report it by module name; it is a template or budget problem,
not a data problem. If the first line is `agenda: nothing due` and you
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

## 3. Is anything stuck in, or rejected from, the outbox?

```bash
find ~/.claude/memory-outbox -name '*.md' 2>/dev/null | wc -l
find ~/.claude/memory-outbox -name '*.rejected' 2>/dev/null
```

Expect 0 and nothing. The outbox holds memories written while the server was
unreachable, and the SessionStart hook drains it. A count that persists across
two sessions means the drain is failing, not that the server was briefly down.

A `.rejected` file is a drain rejection: the kernel refused that write — a
field the schema does not have, a missing module, something that looked like a
credential — and the drain renamed it so it is not retried for ever. It is
never retried, so what it says is not in the record. List each one with the
reason, which is the HTML comment at the end of the file. The fix is to write
it again through the `write` tool with the problem corrected, then delete the
file.

## 4. Is the claim schedule running?

The kernel runs every non-manual claim on a schedule. If the schedule stops,
nothing says so: every goal keeps its last state, and the record looks fine.

Read the end of the `/healthz` line from check 0 — `claims=` — and call the
`claims` tool, whose `last_run` and `interval` say the same thing with the
interval the stamp is judged against.

- A stamp older than two intervals is a failure: the schedule has stopped. Say
  how long ago it last ran.
- `never` is a failure unless the kernel started in the last few minutes,
  because it runs claims as soon as it starts.
- `off` means the deployment turned the kernel's schedule off on purpose. Say
  so, not as a failure. Adapter passes will then be marked `stale`, because
  nothing refreshes them; that is expected under `off`, not a second fault.
- `unknown` from the `claims` tool means this kernel was started without a
  claim runner at all, so it cannot say, even where `/healthz` reads `off`.
  Report it as unknown, not as passing.

## 5. Can every adapter answer?

In the same `claims` output, list every claim whose state is `no-evidence`,
grouped by adapter, with its detail. Each one is a fault in the deployment — a
credential that expired, a source that is unreachable, a query that matches
nothing it can count — never the person being behind on a goal. Name the
adapter as the thing to fix. A manual claim at `no-evidence` is only the person
not knowing yet; leave those out.

## 6. Are working notes dating themselves?

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

Say plainly which of the checks passed. If all pass, one line is enough.

A kernel that does not answer (check 0) is the headline, because it makes the
rest moot. After that, the silent failures lead: a stopped claim schedule
(check 4) above everything else they report, because it leaves every goal's
state quietly out of date; then adapter faults, a module over budget and
rejected outbox writes. Checks 2 and 3's queued count are only meaningful once
check 1 says the guard is live *and* check 0 says the kernel is up: both read
zero on a machine where nothing is happening at all, and a guard that is off
because the kernel is down is expected, not a fault. Check 6 is a list to
review, not a pass or fail.

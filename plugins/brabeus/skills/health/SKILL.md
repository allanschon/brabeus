---
name: health
description: Use when checking whether this machine's brabeus routing is working - the guard is active, nothing is stuck in the outbox, and nothing has accumulated in per-machine scratch. Also use when setting up a new machine.
---

# Is this machine's memory routing working?

Answer for THIS machine only. There is deliberately no way to ask about another
machine: reaching across the network is what made the previous checker
unreliable — it was blind to a sleeping laptop and reported "could not reach it"
as a failure, so it was red most nights for a boring reason.

Run all three checks and give one verdict.

## 1. Is the guard actually refusing?

Presence is not evidence. Exercise it:

```bash
printf '{"tool_name":"Write","tool_input":{"file_path":"%s/.claude/projects/x/memory/probe.md"}}' "$HOME" \
  | "${CLAUDE_PLUGIN_ROOT}/hooks/brabeus-write-guard.sh"
```

Judge this on STDOUT, never on the exit code. Every path in the guard exits 0,
including the denial — a PreToolUse hook refuses by printing a decision, not by
failing. A live guard prints `"permissionDecision":"deny"`. No output at all
means the guard is NOT refusing, and that is the headline of the report.

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

## Reporting

Say plainly which of the three passed. If all three pass, one line is enough.

Do not report a machine healthy on the strength of checks 2 and 3 alone — both
read zero on a machine where nothing is happening at all, so they are only
meaningful once check 1 says the guard is live.

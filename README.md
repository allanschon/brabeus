# Brabeus

Brabeus keeps a private record of who you are and what you're aiming at, and gives it to your AI
assistant at the start of every session. It checks your goals against real evidence and shows you
the gap between what you said matters and what the record shows. You never have to remember to
maintain it, because your assistant raises what needs your attention at the start of a session. It is
one small server, a plugin for the assistant, and a set of modules. It is named for the umpire at
the Greek games.

## Status

The specification is at v1.10 (`docs/personal-context-system-v1.md`). Milestones M0, M1 and M2 are
built. Modules load and validate, and every write names a module and a kind. The context block
renders from the person's records with the agenda's question as its first line. Goals carry claims,
which the kernel checks against a task tracker, a git forge or a date, or asks the person to answer.
The person's confirmed preferences and register reach every session and every subagent it starts as
standing instructions. The interview is a conversation: it drafts the record in the person's words,
confirms each draft through `review`, and closes with a reflection by value. M3, a read-only view,
and M4, the `health` and `finance` modules, are planned. Section 14 of the specification lists what
each milestone delivers.

## What is here

- **The kernel**, in `cmd/brabeus/` and `internal/`: a Go server that keeps a private git
  repository and serves it over MCP, with retrieval that combines keyword and meaning-based
  search.
- **The plugin**, in `plugins/brabeus/`: a Claude Code plugin with a write guard, a drain for
  writes queued while the kernel was unreachable, the MCP server registration, and three skills,
  `/interview`, `/done` and `/health`.
- **The modules**, in `modules/`: `module.json` manifests for the five modules the specification
  ships, `memory`, `identity`, `telos`, `health` and `finance`. The kernel loads and validates the
  enabled set when it starts, enforces each module's kinds on every write, and renders the
  ratified-record modules into the context block.
- **The documents**, in `docs/`: the specification, a set of C4 diagrams, a plain-language
  description and a domain-driven design analysis.

## Running the kernel

```sh
docker build -t brabeus .
docker run -d --name brabeus --restart always \
  -p 127.0.0.1:8082:8082 \
  -e BRABEUS_REPO=ssh://git@example.org/you/record.git \
  -e BRABEUS_TRUSTED_PROXIES=172.17.0.1 \
  -v /etc/brabeus/deploy_key:/etc/brabeus/deploy_key:ro \
  -v brabeus-data:/var/lib/brabeus \
  brabeus
```

`BRABEUS_REPO` is the one required value: the private repository the kernel writes your record
to. `.env.example` lists every other variable and its default.

The kernel also serves a read-only view of the record at `/view/`, behind the same identity and
authentication as `/context`. Opening `http://<the kernel's front>/view/` in a browser on a machine the
kernel can identify shows the home page, with one page per module behind it, and no other set-up is
needed. The view cannot write.

## Tools

The tools call a record a memory, a name that predates modules.

- `list` returns records with their module, kind, scope and one-line description.
- `read` returns one record in full, or a file from the notebook, the person's own notes. Only the
  person's own sessions can read the notebook.
- `search` ranks records by relevance, by keyword and, when an embedding sidecar is running, by
  meaning; the notebook is searched by keyword only unless `BRABEUS_NOTEBOOK_EMBED` is set, in
  which case it gets the same dense leg. It searches working-memory records by default, because ratified records already reach
  every session through the context block. `profile: ratified-record` or `all` widens the search,
  and naming a module widens it for that module alone.
- `write` saves a record.
- `delete` removes a record.
- `context` returns the session context block, described below.
- `review` answers one agenda question by confirming, correcting, retiring or snoozing a
  ratified record. It is the only operation that moves a record's `reviewed` date, so a
  `reviewed` date always means the person was asked and answered.
- `modules` returns the enabled modules the caller may read, with each kind's fields and
  interview questions.
- `claims` lists the claims on the person's goals with their latest results, and marks a pass
  that is older than two check intervals as stale.
- `claim_result` records the person's answer to a `manual` claim. It refuses a claim that an
  adapter checks, because that result comes from the kernel's own run.
- `reflect` returns the gap by value: each value, the goals that serve it with their claim states
  and days since they were confirmed, and the goals that serve no value.

`read` and `search` reach the notebook with the argument `repo: notebook` (`repo: projects` still
works, the name it used before M2).

## The context block

The context block is one rendered block of at most 2 KB. It starts with the agenda's top question,
followed by each enabled ratified-record module's summary, in priority order. Each module has a
byte budget within the block. A module over its budget is refused rather than truncated: it
renders one line saying so and reports a fault, so a cut is always visible instead of silently
dropping part of the record. The agenda line is handled the same way when it has to be cut.

The block is filtered to the calling machine's scope, and it may lag a write made on another
machine by up to one sync interval, which is 15 minutes. The plugin fetches it at session start
over plain HTTP at `GET /context`, which sits behind the same identity and authentication as
`/mcp`.

A caller resolved as a consumer, meaning one named in `BRABEUS_CONSUMERS`, sees only modules whose
manifest declares `audience: any`. That holds on every read path: search, read, list, the
vocabulary and the block. A consumer is refused the notebook entirely, because the notebook has no
modules and so no audience of its own. A consumer never writes, deletes or reviews. A caller the
kernel cannot identify at all is not a consumer — it is refused outright, with a 403, before any
tool runs (spec §11).

## Instructions

A ratified-record kind marked as instructions makes the confirmed records of that kind the
person's standing instructions for how to work with them. The shipped `identity` module marks
`preference` and `register`. Two manifest keys declare it (spec §6):

- A kind sets `"instructions": true`. It must list `"source"` among its `optional` fields, which
  is where the interviewer's label goes, because the record's body is delivered word for word.
- The module sets `"instructions_budget_bytes"`, the size its instructions should stay within.
  `identity` sets 4096.

Only a `ratified-record` module may declare either key. A module that marks a kind must declare a
positive budget, and a module that marks none must not declare one. A deployment with no marked
kind delivers nothing.

Every session receives the confirmed, unretired records of those kinds, scope-filtered as the
block is, in module priority and then by path. Each record contributes its body, trimmed, or the
value of the kind's first field when the body is empty. The text opens with a line saying that
where the task an agent was given conflicts with an instruction, the instruction wins on anything
that cannot be undone or reaches beyond the working copy, and the task wins on anything else.
Drafts are not delivered. A consumer receives only the instructions of modules whose audience is
`any`, so `identity`'s reach only the person's own sessions.

The kernel serves them at `GET /instructions`, behind the same identity and authentication as
`/context`; the `context` tool reports each module's instructions size against its budget.

Over its budget, a module's instructions are still delivered in full. The excess is a `budget`
item, the last on the agenda, after onboarding: "The identity instructions every session receives
are 5120 of 4096 bytes. Which can be merged or retired?" A `later` on it is not recorded, and
`/health` reports it. Confirming or correcting an instruction records the delivered text in the
review's commit, under `Delivered:`.

The plugin's `SessionStart` hook fetches the instructions after it drains the outbox and puts
them in the session's context after the block. It saves them as that session's copy in the
plugin's data directory, which the harness sets for hooks:
`~/.claude/plugins/data/<plugin>-<marketplace>/instructions/<session id>.json`, mode 600. The
`SubagentStart` hook gives every subagent that session's copy, with no network call; a subagent
whose session has no copy is told the instructions are unavailable. If the kernel is unreachable
at start, the newest copy for the same machine and project, made from the same kernel, stands in,
labelled with its date and saved as the session's own. Copies older than 30 days are deleted,
except the newest for each machine and project.

The harness replaces a hook's text of more than 10,000 characters with a path to a file the model
is not asked to read, so the plugin keeps each injection under 9,800 characters. Past that it
drops whole records from the end and names them on a last line, `Not delivered, over the hook's
limit:`, which the `/health` skill also reports, reading it from the saved copy.

The write guard refuses the assistant's Write, Edit and NotebookEdit on the saved copies and its
common Bash write forms, whatever the kernel's profiles say. That stops an accidental write of text
the person never confirmed; the Bash patterns can be walked around deliberately, so it is not a
barrier against a determined bypass.

## Claims

A goal may carry claims: short statements of what true would look like, each naming the evidence
that would show it (spec §8.1).

```yaml
claims:
  - text: "At least three articles published this quarter"
    check: { adapter: tracker, done: true, label: article, since: 2026-07-01, min: 3 }
  - text: "The first draft of the guide is written"
    by: 2026-11-15
    effort: 21d
    check: { adapter: manual }
  - text: "Every photo from 2025 has been reviewed"
    check: { adapter: manual, of: 1200, since: 2026-09-01 }
  - text: "The side project has a commit in the last fortnight"
    standing: true
    check: { adapter: forge, repo: side-project, since: -14d, min: 1 }
  - text: "The target date still holds"
    standing: true
    check: { adapter: manual }
```

A module declares which adapters its goals may name. The kernel checks every claim's arguments
when the goal is written, so a goal never carries a claim that no run could evaluate.

| adapter | required | optional | rule |
|---|---|---|---|
| `tracker` | `label`, `since`, `min` | `done`: `true` (default) counts done tasks, `false` counts open ones | `since` is `YYYY-MM-DD` or `-Nd` with N of at least 1; `min` is an integer of at least 1; no other key |
| `forge` | `repo`, `since`, `min` | `merged`: `true` counts merged pull requests instead of commits | `repo` is `owner/name`, or a bare `name` that resolves against `BRABEUS_FORGE_OWNER`; `since` and `min` as above |
| `date` | one or both of `before`, `after` | — | each `YYYY-MM-DD`; `after` earlier than `before` when both are given |
| `manual` | — | `of` and `since` together: `of` is the total, an integer of at least 1, and `since` is `YYYY-MM-DD` | without them it is a yes-or-no claim, asked at interview; with them each answer carries a count |

No argument value may contain a comma, because the check map is split on commas.

These keys sit beside `text` in the claim, because they say what the claim means:

| key | value | rule |
|---|---|---|
| `standing` | `true` | the claim should hold all the time, so one that does not hold is a contradiction now. A `date` claim is always standing. A claim with a rolling `since` (`-14d`) must say `standing: true`. A standing claim takes no `by` and no `effort` |
| `by` | `YYYY-MM-DD` | the claim's deadline, no later than the goal's `by`. Without it an end-state claim takes the goal's `by` |
| `effort` | `<n>d`, n of at least 1 | the person's estimate of the calendar days the work will take, for a yes-or-no claim only. There is no default: without it a yes-or-no claim gives no early warning |

A claim without `standing` is an end-state claim, true by its deadline. Deadlines are calendar dates
in the kernel's `TZ`, UTC when unset.

A result is stored as what was measured, one of three states, and for a counting claim the count and
the target it was compared with:

| measured | means |
|---|---|
| `pass` | the adapter returned evidence and the claim held |
| `fail` | the adapter returned evidence and the claim did not hold |
| `no-evidence` | the adapter could not answer |

What a claim means today is derived whenever it is read, from the measurement, the claim's keys and
the date, so a claim moves with the calendar and not only at a run. `open` and `behind` are
derived and never written to a results file.

| state | when |
|---|---|
| `pass` | the evidence holds |
| `open` | an end-state claim not met yet, with its deadline ahead and its work on pace |
| `behind` | an end-state claim not met yet whose pace says it may miss its deadline: a paced claim whose count is below half the work its window so far would expect, counted in whole items, with the expected work and its half each rounded down, or a yes-or-no claim with an `effort` and no more days left than that effort, counting the deadline day as one |
| `fail` | a standing claim not met, or an end-state claim still not met after its deadline day |
| `no-evidence` | as measured: a fault in the deployment |
| `unchecked` | a standing claim with nothing measured yet |

Only `fail` is a contradiction, and `behind` is the warning before one. A refused credential, a
repository or tracker the backend does not have, an unreachable host and an unreadable answer are
all `no-evidence`, with the reason as the detail. The same applies to a tracker label that appears on no task at all. A claim naming an
adapter this deployment has not configured also reads `no-evidence`, and the detail says why.

Each deployment configures its backends with these variables:

- `BRABEUS_TRACKER`: `vikunja`, or empty for no tracker.
- `BRABEUS_TRACKER_URL`: the tracker's base URL. It is required when a tracker is named.
- `BRABEUS_TRACKER_TOKEN`: a Vikunja API token with the Tasks -> Read All permission, which is
  what the adapter's listing query needs.
- `BRABEUS_FORGE`: `gitea`, `github`, or empty for no forge.
- `BRABEUS_FORGE_URL`: the forge's base URL. It is required for Gitea. For GitHub it defaults to
  `https://api.github.com`.
- `BRABEUS_FORGE_TOKEN`: a read-only token for the repositories that claims name.
- `BRABEUS_FORGE_OWNER`: the owner a bare `repo:` name resolves against. If it is empty, such a
  claim reads `no-evidence`.

An unknown backend name, or a named backend without its URL, stops the kernel at startup.

The kernel runs every non-manual claim when it starts and then every `BRABEUS_CLAIM_INTERVAL`
(`24h` by default; `0` turns the schedule off). A run commits only when a claim's state changes,
and never touches the goal itself, so a goal's `updated` stamp and history stay what the person
made them. `/healthz` ends with `claims=`: `off`, `never`, or the time of the last run, which
survives a restart. A `pass` older than two intervals is shown as stale, so one missed run changes
nothing and a stopped schedule shows within two intervals.

A goal whose results the run could not record — a results file that no longer parses, a detail an
adapter refuses, a goal deleted mid-run — does not stop the run: the kernel logs it, counts it, and
carries on so one broken goal does not make every other goal's fresh result look stale too. That
count survives a restart alongside the stamp, and `/healthz` appends `errors=<n>` after `claims=`
when it is non-zero, so a goal failing on every run is visible on this line and not only in the
kernel's log. A clean run adds nothing to the line.

## Upgrading a record from before modules

A record written before modules existed carries `type` instead of `module` and `kind`. Setting
`BRABEUS_MIGRATE=1` for one start retags every such file through the enabled memory module's
`legacy_types`, in a single commit, leaving paths and each file's `updated` stamp unchanged. A file
that maps to nothing aborts the whole run before anything is written, and a second run with the
flag still set finds nothing to do.

Rehearse on a clone of the record first. Run the kernel against it with the flag unset and save a
set of search results; then run it again with `BRABEUS_MIGRATE=1` and compare the same searches.
They should be identical, because a retag changes only frontmatter keys that ranking never scores.
Unset the flag once the live run has happened. Leaving it set makes the migration something every
restart does rather than something you did once.

## Installing the plugin

```sh
claude plugin marketplace add allanschon/brabeus
claude plugin install brabeus
```

Then set `BRABEUS_URL` to your kernel's base address: scheme, host and port, with no path. The
plugin's `.mcp.json` appends `/mcp` to it for the tools, and the SessionStart hook appends
`/healthz` and `/context`. Before relying on the hooks on a new machine, run
`bash plugins/brabeus/hooks/test.sh`, which checks that the machine has the tools they need: bash,
jq, curl and python3.

Set `BRABEUS_TOKEN` if your kernel identifies callers by token; the hooks and the MCP registration
send it as `Authorization: Bearer $BRABEUS_TOKEN`. Leave it unset where the network itself
identifies the caller. The hooks then send no header, and the MCP registration sends an empty
`Bearer `, which a kernel that does not use tokens ignores.

Every session start fetches the kernel's context block and puts it first in the session's
context, with a three-sentence routing reminder under it. If the kernel is unreachable, the
session still starts, without the block.

The write guard denies writing memory into the assistant's per-machine scratch directory, but only
where the kernel reports a working-memory module enabled for this session. When the kernel is
unreachable or reports none, scratch is allowed, so that a session is never left with neither the
shared store nor a local fallback.

The plugin ships three skills. `/interview` holds the conversation in section 9 of the
specification: it gets to know the person or checks in with them, drafts what they say, confirms
each draft through `review` in their own words, and closes with a reflection by value. `/done`
writes a done-statement for a piece of work before it starts (section 8.2). `/health` reports
whether the kernel and the guard are working on this machine.

## Documents

- [Specification](docs/personal-context-system-v1.md)
- [C4 diagrams](docs/personal-context-system-c4.md)
- [Plain-language description](docs/personal-context-system-plain.md)
- [Domain-driven design](docs/ddd/README.md)

## Name

Brabeus, βραβεύς, was the umpire at the Greek games, who judged honestly and awarded the prize.

## Licence

Apache 2.0. See [LICENSE](LICENSE).

## Contributing

Brabeus is not yet accepting contributions; see [CONTRIBUTING.md](CONTRIBUTING.md).

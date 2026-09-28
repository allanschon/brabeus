# Brabeus

Brabeus keeps a private record of who you are and what you're aiming at, and gives it to your AI
assistant at the start of every session. It checks your goals against real evidence and shows you
the gap between what you said matters and what the record shows. You never have to remember to
maintain it, because the system raises what needs your attention at the start of a session. It is
one small server, a plugin for the assistant, and a set of modules. It is named for the umpire at
the Greek games.

## Status

The specification is at v1.7 (`docs/personal-context-system-v1.md`). Milestones M0 and M1 are
built: modules load and validate, every write names a module and a kind, and the context block
renders from the person's records with the agenda's question as its first line. M2 adds evidence
checks on goals and the conversational interview. Section 14 of the specification lists what each
milestone delivers.

## What is here

- **The kernel**, in `cmd/brabeus/` and `internal/`: a Go server that keeps a private git
  repository and serves it over MCP, with retrieval that combines keyword and meaning-based
  search.
- **The plugin**, in `plugins/brabeus/`: a Claude Code plugin with a write guard, a drain for
  writes queued while the kernel was unreachable, the MCP server registration, and two skills,
  `/health` and `/interview`.
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

## Tools

The tools call a record a memory, a name that predates modules.

- `list` returns records with their module, kind, scope and one-line description.
- `read` returns one record in full, or a file from the notebook, the person's own notes. Only the
  person's own sessions can read the notebook.
- `search` ranks records by relevance, by keyword and, when an embedding sidecar is running, by
  meaning; the notebook is searched by keyword only. It searches working-memory records by default, because ratified records already reach
  every session through the context block. `profile: ratified-record` or `all` widens the search,
  and naming a module widens it for that module alone.
- `write` saves a record.
- `delete` removes a record.
- `context` returns the session context block, described below.
- `review` answers one agenda question by confirming, correcting, retiring or snoozing a
  ratified record. It is the only operation that moves a record's `reviewed` date, so a
  `reviewed` date always means the person was asked and answered.

`read` and `search` reach the notebook with the argument `repo: projects`.

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

Set `BRABEUS_TOKEN` if your kernel identifies callers by token; the hooks send it as
`Authorization: Bearer $BRABEUS_TOKEN`. Leave it unset where the network itself identifies the
caller, and no header is sent.

Every session start fetches the kernel's context block and puts it first in the session's
context, with a three-sentence routing reminder under it. If the kernel is unreachable, the
session still starts, without the block.

The write guard denies writing memory into the assistant's per-machine scratch directory, but only
where the kernel reports a working-memory module enabled for this session. When the kernel is
unreachable or reports none, scratch is allowed, so that a session is never left with neither the
shared store nor a local fallback.

The plugin ships two skills. `/health` reports whether the kernel and the guard are working on
this machine. `/interview` asks the top item on the kernel's agenda and records the person's
answer through `review`; section 9 of the specification describes the conversation that replaces
it in M2.

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

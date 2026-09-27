# Brabeus

A private record of who you are and what you're aiming at, kept so your AI assistant reads it at
the start of every session, checks your goals against real evidence, and reflects the gap back —
without you having to remember to maintain it. One small server, a plugin, and modules. Named for
the umpire at the Greek games.

## Status

Specified at v1.5 (`docs/personal-context-system-v1.md`). M1 in progress: modules load and
validate; every write names a module and a kind. See §14 for what comes next.

## What is here

- **`cmd/brabeus/` and `internal/` — the kernel (Go)** — a Go server serving a private git
  repository over MCP with hybrid (lexical + dense) retrieval.
- **The plugin** — a Claude Code plugin: a routing guard, an offline outbox drain, MCP server
  registration and a `/health` skill.
- **The modules** — `module.json` manifests for the five modules the spec ships (`memory`,
  `identity`, `telos`, `health`, `finance`). The kernel loads and validates the enabled set on
  startup; enforcing kinds on writes and rendering the context block follow in M1.
- **The documents** — the full specification, a C4 diagram set, and a plain-language description.

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

`BRABEUS_REPO` is the one required value — the private repository the kernel writes your record
to. See `.env.example` for every other variable and its default.

## Tools

- **`list`** — memories with their module, kind, scope and one-line description.
- **`read`** — one memory in full, or a `projects` mirror file (the mirror only for the person's
  own sessions).
- **`search`** — by relevance, lexical and (with a sidecar) meaning-based. Searches working-memory
  records by default; `profile: ratified-record` or `all` widens it, and naming a module widens it
  for that module alone.
- **`write`** — save a memory.
- **`delete`** — remove a memory.
- **`context`** — the session context block (below).
- **`review`** — answer one agenda question, confirming, correcting, retiring or snoozing a
  ratified record. The only operation that moves a record's `reviewed` date.

## The context block

One rendered block, at most 2 KB: the agenda's top question first, then each enabled
ratified-record module's summary, in priority order. A module over its byte budget is refused,
not truncated — it renders one line saying so and reports a fault, same as the agenda line
being cut. It is scope-filtered to the caller's own machine and may lag another machine's write
by up to one sync interval (15 minutes). It is also served over plain HTTP at `GET /context`,
behind the same identity and auth as `/mcp`, for the plugin's session start.

A caller resolved as a **consumer** — named in `BRABEUS_CONSUMERS`, or unresolved — sees only
modules whose manifest declares `audience: any`, on every read path: search, read, list, the
vocabulary and the block, and the `projects` mirror is refused to it entirely. A consumer never
writes, deletes or reviews.

## Upgrading a record from before modules

A record written before modules existed carries `type` instead of `module` and `kind`. Setting
`BRABEUS_MIGRATE=1` for one start retags every such file through the enabled memory module's
`legacy_types`, in a single commit, with paths and each file's `updated` stamp unchanged. A file
that maps to nothing aborts the whole run before anything is written, and a second run with the
flag still set finds nothing to do.

Rehearse on a clone of the record first: run the kernel against it with the flag unset, save a
set of search results, run it again with `BRABEUS_MIGRATE=1`, and diff the same searches after.
They should be identical — a retag changes only frontmatter keys the ranking never scores.
Unset the flag once the live run has happened; leaving it set makes the migration a thing every
restart does rather than a thing you did once.

## Installing the plugin

```sh
claude plugin marketplace add allanschon/brabeus
claude plugin install brabeus
```

Then set `BRABEUS_URL` to your kernel's base address (scheme, host, port — no path). The
plugin's `.mcp.json` appends `/mcp` to it for the tools, and the SessionStart hook appends
`/healthz` and `/context`. `bash plugins/brabeus/hooks/test.sh` checks a machine has the
tooling the hooks need (bash, jq, curl, python3) before you rely on them there.

Set `BRABEUS_TOKEN` if your kernel runs token identity mode; the hooks send it as
`Authorization: Bearer $BRABEUS_TOKEN`. Leave it unset where identity comes from the
network itself instead — no header is sent.

Every session start fetches the kernel's `/context` block and puts it first in the
session's context, with a three-sentence routing reminder under it; if the kernel is
unreachable the session still starts, just without the block. The write guard denies writing memory
into per-machine scratch only where the kernel reports a working-memory module enabled for
this session — when the kernel is unreachable or reports none, scratch is allowed rather
than leaving a session with neither the shared store nor a local fallback.

Two skills ship with the plugin: `/health` reports whether the kernel and guard are
working on this machine, and `/interview` asks the top item on the kernel's agenda and
records the person's answer.

## Documents

- [Specification](docs/personal-context-system-v1.md)
- [C4 diagrams](docs/personal-context-system-c4.md)
- [Plain-language description](docs/personal-context-system-plain.md)
- [Domain-driven design](docs/ddd/README.md)

## Name

Brabeus — βραβεύς, the umpire at the games: judges honestly, awards the prize.

## Licence

Apache 2.0. See [LICENSE](LICENSE).

## Contributing

Not yet accepting contributions — see [CONTRIBUTING.md](CONTRIBUTING.md).

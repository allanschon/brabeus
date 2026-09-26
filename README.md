# Brabeus

A private record of who you are and what you're aiming at, kept so your AI assistant reads it at
the start of every session, checks your goals against real evidence, and reflects the gap back —
without you having to remember to maintain it. One small server, a plugin, and modules. Named for
the umpire at the Greek games.

## Status

Specified at v1.4 (`docs/personal-context-system-v1.md`). Nothing beyond the kernel is built yet.
This is milestone M0: the public repository seeded with the kernel, the plugin, the five module
manifests, a synthetic test corpus and the specification itself. See §14 for what comes next.

## What is here

- **The kernel** — a Go server, root package, serving a private git repository over MCP with
  hybrid (lexical + dense) retrieval.
- **The plugin** — a Claude Code plugin: a routing guard, an offline outbox drain, MCP server
  registration and a `/health` skill.
- **The modules** — manifests for the five modules the spec ships (`memory`, `identity`, `telos`,
  `health`, `finance`), as documents of intent; the module contract itself is not yet enforced.
- **The documents** — the full specification, a C4 diagram set, a plain-language description, and
  a systems analysis.

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

## Installing the plugin

```sh
claude plugin marketplace add allanschon/brabeus
claude plugin install brabeus
```

Then set `BRABEUS_URL` to your kernel's address — the plugin's `.mcp.json` reads it from the
environment.

## Documents

- [Specification](docs/personal-context-system-v1.md)
- [C4 diagrams](docs/personal-context-system-c4.md)
- [Plain-language description](docs/personal-context-system-plain.md)
- [Systems analysis](docs/personal-context-system-v1-systems-analysis.md)

## Name

Brabeus — βραβεύς, the umpire at the games: judges honestly, awards the prize.

## Licence

Apache 2.0. See [LICENSE](LICENSE).

## Contributing

Not yet accepting contributions — see [CONTRIBUTING.md](CONTRIBUTING.md).

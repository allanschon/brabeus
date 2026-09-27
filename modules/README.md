# Modules

A module is a directory with a manifest, `module.json`. It declares a set of record kinds (each
with its required and optional fields and a freshness threshold), an interview prompt per kind, a
summary template that renders the module's part of the context block, the evidence adapters its
claims may use, and which of the kernel's two fixed profiles it runs under —
`working-memory` or `ratified-record`. The profile, not the manifest, decides who may write, and
whether a record needs to be ratified before it renders. See `docs/personal-context-system-v1.md`
§6 for the full contract.

Two things a manifest cannot say, and where they are instead: a `working-memory` module has no
budget and no summary template, because its records are searched and never rendered into the
context block; and a core module's audience is `self` whatever the manifest says. The kernel
refuses to start on either.

Two optional keys per kind beyond the spec's example: `first`, the question the interview asks
when nothing of that kind is on file yet, and `timeless`, which exempts a kind from the
working-memory freshness lint. A ratified-record module lists in `onboarding` the kinds to ask
for, in order, when none of that kind is on file; each must carry a `first` question. Two optional keys on a working-memory module: `layout`, `free` or
`kind`, which says whether the path rule is the store's own tree or `<module>/<kind>/<slug>.md`;
and `legacy_types`, the map the one-time migration uses to give pre-module records a kind.

The kernel loads and validates these on startup and refuses a set it does not understand; it
enforces the kinds on writes, and rendering the summaries lands with the context block.

Five modules ship: `memory` under `working-memory` — the assistant's per-machine working notes,
and the only module with scope keys — and four core modules under `ratified-record`: `identity`,
`telos`, `health` and `finance`. See §7 for what each holds.

Each of the four ratified-record modules ships `summary.md.tmpl`, a `text/template` over
`internal/block.Data`. The custom functions are `first`, `date` and `age`; `text/template`'s own
builtins (`index`, `or`, `printf` and the rest) are available as always. Nothing else is added
deliberately.

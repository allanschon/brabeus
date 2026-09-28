# Modules

A module is a directory with a manifest, `module.json`. The manifest declares a set of record
kinds, each with its required and optional fields, a freshness threshold and an interview prompt.
It also declares a summary template that renders the module's part of the context block, the
evidence adapters its claims may use, and which of the kernel's two fixed profiles it runs under,
`working-memory` or `ratified-record`. The profile, not the manifest, decides who may write and
whether a record must be confirmed by the person before it renders as theirs. Keeping those rules
out of the manifest means no module can loosen them. Section 6 of the specification is the full
contract.

Two things a manifest cannot say, and the kernel refuses to start if one tries:

- A `working-memory` module has no budget and no summary template, because its records are
  searched and never rendered into the context block.
- A core module's audience is `self`, whatever the manifest says, so that no configuration can
  make the person's own record readable by another program.

A kind may carry two optional keys. `first` is the question the interview asks when nothing of
that kind is on file yet. `timeless` exempts a kind from the working-memory freshness lint, which
lists any record that is neither timeless, dated nor a pointer. A
ratified-record module lists in `onboarding` the kinds to ask for, in order, when none of that kind
is on file, and each kind it names must carry a `first` question.

A working-memory module may carry two more. `layout` says whether the module's paths follow the
store's own tree (`free`) or `<module>/<kind>/<slug>.md` (`kind`). `legacy_types` is the map the
one-time migration uses to give records written before modules a kind.

Specification v1.6 adds `lenses` and `draft` per kind and `intro` per module, for the
conversational interview. The kernel accepts them from M2; until then it refuses them, as it
refuses any key it does not know.

The kernel loads and validates the manifests when it starts and refuses a set it does not
understand. It enforces each module's kinds on every write and renders each ratified-record
module's summary into the context block.

Five modules ship. `memory` runs under `working-memory`: it holds the assistant's working notes,
and it is the only module with scope keys. The four core modules, `identity`, `telos`, `health` and
`finance`, run under `ratified-record`. Section 7 of the specification describes what each holds.

Each of the four ratified-record modules ships `summary.md.tmpl`, a `text/template` over
`internal/block.Data`. The kernel adds three functions, `first`, `date` and `age`, and adds no
others; `text/template`'s own built-in functions, such as `index`, `or` and `printf`, are available
as usual.

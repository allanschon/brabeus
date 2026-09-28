# Modules

A module is a directory with a manifest, `module.json`. The manifest declares a set of record
kinds, each with its required and optional fields and, usually, a freshness threshold and an
interview prompt.
It also declares a summary template that renders the module's part of the context block, the
evidence adapters its claims may use, and which of the kernel's two fixed profiles it runs under,
`working-memory` or `ratified-record`. The profile, not the manifest, decides who may write and
whether a record must be confirmed by the person before it renders as theirs. Keeping those rules
out of the manifest means no module can loosen them. Section 6 of the specification is the full
contract.

Two things a manifest cannot say, and the kernel refuses to start if one tries:

- A `working-memory` module has no budget and no summary template, because its records are
  searched and never rendered into the context block.
- A core module's audience is `self`, and a manifest that declares otherwise is refused, so that
  no configuration can make the person's own record readable by another program.

A ratified kind's `freshness_days` and `interview` are optional. A kind without `freshness_days`
is never due because of its age; its records still come up as drafts, through a failed claim, or
through a `due_field`. A kind without `interview` is asked "Is this still right?" when a record
goes stale, so a module never produces an empty question. From M2 a kind may also name a
`due_field`, a date field after which a record is due if it has not been reviewed since; `telos`
uses it for a decision's `revisit` date, because a decision is better asked on the date the person
chose than after a fixed age.

A kind may carry two more optional keys. `first` is the question the interview asks when nothing of
that kind is on file yet. `timeless` exempts a kind from the working-memory freshness lint, which
lists any record that is neither timeless, dated nor a pointer. A
ratified-record module lists in `onboarding` the kinds to ask for, in order, when none of that kind
is on file, and each kind it names must carry a `first` question.

A working-memory module may carry two more. `layout` says whether the module's paths follow the
store's own tree (`free`) or `<module>/<kind>/<slug>.md` (`kind`). `legacy_types` is the map the
one-time migration uses to give records written before modules a kind.

The specification adds `lenses` and `draft` per kind and `intro` per module, for the
conversational interview. The kernel validates their shape and attaches no meaning to them.

The kernel loads and validates the manifests when it starts and refuses a set it does not
understand, because an ignored key is a declaration the author believes is in force and is not: a
misspelling, or a key from a newer kernel, would otherwise change behaviour silently. It enforces each module's kinds on every write and renders each ratified-record
module's summary into the context block.

Five modules ship. `memory` runs under `working-memory`: it holds the assistant's working notes,
and it declares `scope_keys`, so the plugin fills in the machine and project when the assistant
writes. Every record carries a scope, `global` by default; in this version only a working-memory
module may declare `scope_keys`. The four core modules, `identity`, `telos`, `health` and
`finance`, run under `ratified-record`. Section 7 of the specification describes what each holds.

Each of the four ratified-record modules ships `summary.md.tmpl`, a `text/template` over
`internal/block.Data`. The kernel adds three functions, `first`, `date` and `age`, and
deliberately no others; `text/template`'s own built-in functions, such as `index`, `or` and
`printf`, are available as usual. The set is closed because a template ships with a module and must
only format the records it is given: a function that could read the environment or a file would
let a module template print a deployment's secrets into the context block. A new function is a
kernel change, reviewed like one.

Every record a template ranges over is a `block.Rec`, the stored record plus `.Mark`:
`" (unconfirmed)"` until the record is reviewed, and `""` after (spec §10). A shipped template
writes `{{.Mark}}` at the end of every line that names a record, so a draft can never render as
the person's confirmed word.

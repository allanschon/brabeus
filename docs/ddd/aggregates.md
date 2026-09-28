# Aggregates

An aggregate is a cluster of objects that changes as one unit, behind one root, inside one
consistency boundary. Brabeus has two: the record and the module set. The agenda item and the
context block look like aggregates and are not; they are computed views. Terms are the
[glossary's](ubiquitous-language.md); the rules each aggregate enforces are listed in
[`invariants.md`](invariants.md).

## Record

|            |                                                                                                                                                         |
| ---------- | ------------------------------------------------------------------------------------------------------------------------------------------------------- |
| root       | the record: one markdown file (`store.Stored`)                                                                                                          |
| identity   | its path in the store, canonicalised (`store.MemoryPath`)                                                                                               |
| inside     | content (`store.Record`: name, description, module, kind, id, scope, fields, body) and stamps (`store.Meta`: `updated`, `reviewed`, `retired`, snoozes) |
| repository | the store (`store.Store`): `Write`, `Review`, `Delete`, `Records`, `Search`, `Migrate`                                                                  |
| context    | Record, with Ratification's verdicts applied through `Store.Review`                                                                                     |

The record is an entity: a value is reworded, a goal's date moves, and it is still the same record
at the same path. Its content and its stamps are value objects, replaced whole on each write.

Two operations change a record, and they own different halves of it. A write sets content and
moves `updated`; it carries the other stamps through unchanged. A review applies a verdict: it
moves `reviewed`, `retired` or the snooze count, and on `corrected` replaces content too. No
other path changes a record except `Delete`, which removes it, and the one-time migration, which
retags it.

A record in a ratified-record module has a lifecycle, and its stamps are the state:

| state    | stamps                      | entered by                           | rendered             |
| -------- | --------------------------- | ------------------------------------ | -------------------- |
| draft    | `reviewed` zero             | a write                              | marked unconfirmed   |
| ratified | `reviewed` set, not retired | a review, `confirmed` or `corrected` | as the person's word |
| retired  | `retired` set               | a review, `retired`                  | never                |

A `later` verdict leaves the state as it was and counts a snooze. A write to a draft keeps it a
draft; the interview rewrites drafts freely until the person confirms one (spec §9). A write to a
ratified record also keeps its state, which is the gap described in [`invariants.md`](invariants.md).

A `memory/thread` is a record like any other. Its `belongs_to` field is a guess at a module, not a
reference: the module need not exist, and the thread is matched to one by meaning when a module is
enabled. It ends by deletion once a module that covers it holds a draft developed from it
(spec §7).

**The consistency boundary is the store, not the record.** Every change takes the store's single
lock, pulls, rewrites the record and the index line in `MEMORY.md`, commits and pushes. The index
is a projection of every record, and keeping it exact inside the same commit is what makes the
transaction store-wide. At one writer and a few hundred records that costs nothing; it would be
the first thing to revisit if the kernel ever served more than one writer.

Records refer to each other by identifier, never by containment: a goal names the values it
`serves`. The crossing kind is not a reference at all. A
`memory/preference` and the `identity/preference` it becomes are the same file, and ratification
changes which module governs it, not where it lives.

**Claims (M2) will live inside the goal record** — spec §8.1 records a claim's results "on the
goal" — so the goal is their aggregate root. A result written to the goal moves its `updated`
stamp, and the revision line compares `updated` against `reviewed`. As built, every scheduled claim
run would therefore mark its goal "revised since the last review". Either results move a stamp of
their own, or the revision line has to look at content rather than stamps.

## Module set

|          |                                                                                                                                                                               |
| -------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| root     | the module set (`module.Set`)                                                                                                                                                 |
| identity | the deployment: one set per kernel instance, loaded at start                                                                                                                  |
| inside   | manifests (`module.Manifest`), each with its kinds (`module.Kind`), its mode's bundle (`module.Bundle`) and, from M2, its `intro` and each kind's `lenses` and draft question |
| factory  | `module.Load`, which validates every manifest and then the set as a whole                                                                                                     |
| context  | Schema                                                                                                                                                                        |

The module set is created once, validated whole, and never changed while the kernel runs. That
makes it closer to an immutable value than to an entity: a different set means a restart. Its
invariants are the ones only the whole set can check — budgets summing under the cap — and the
ones each manifest checks alone, which `Load` runs before accepting any.

The mode and its bundle are value objects. The bundle is the published statement of a mode's rules;
the kernel's contexts check a module's mode directly instead of reading it (see
[`bounded-contexts.md`](bounded-contexts.md)).

## Not aggregates

| looks like one                         | is                                   | why                                                                                                                                                                                                               |
| -------------------------------------- | ------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| agenda item (`agenda.Item`)            | a value in a read model              | computed from records and the module set on every request by `agenda.Compute`; never stored, so there is nothing to keep consistent                                                                               |
| context block                          | a value produced by a domain service | rendered by `block.Renderer.Render` from records, the agenda and the module set; its rules (the cap, budgets, faults) are rules of rendering, not of stored state                                                 |
| review input (`store.Answer`)          | a value object carried by a command  | the question asked, the verdict, and for `corrected` the new body and fields; from M2 also the person's answer in their own words. Today the question is passed beside it. It exists for the length of one review |
| caller                                 | a value object                       | a machine name and whether it is a consumer, resolved once per request                                                                                                                                            |
| forbidden modules (`store.Visibility`) | a value object                       | derived per session from the caller and the module set                                                                                                                                                            |
| fault (`block.Fault`)                  | a value object                       | reported with the block it describes and not kept                                                                                                                                                                 |

## Domain services

Operations that belong to no single aggregate, each a function of its inputs:

| service                 | does                                                                                        |
| ----------------------- | ------------------------------------------------------------------------------------------- |
| `agenda.Compute`        | orders what is due, from records and the module set, each item with its reason and question |
| `block.Renderer.Render` | renders the context block within the cap and the budgets                                    |
| `module.Set.RuleFor`    | names the governing module and kind for a record, which is how the crossing kind works      |
| `server.RenderContext`  | the one path to the block for a caller: forbidden modules, scope, agenda, render            |

# Aggregates

An aggregate is a cluster of objects that changes as one unit, behind one root, inside one
consistency boundary. Brabeus has two: the record and the module set. The agenda item and the
context block look like aggregates but are not, because nothing about them is stored: both are
computed from the records and the module set each time they are asked for. Terms are the
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

The record is an entity: when a value is reworded or a goal's date moves, it is still the same
record at the same path. Its content and its stamps are value objects, replaced whole on each
write rather than edited in place.

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

A `later` verdict leaves the state as it was and counts a snooze, so a deferral stays visible
(spec §9). A write to a draft keeps it a draft, which is what lets the interview rewrite drafts
freely until the person confirms one (spec §9). A write to a ratified record also keeps its state,
so reworded content can go on reading as confirmed; [`invariants.md`](invariants.md) describes that
gap.

A `memory/thread` is a record like any other. Its `belongs_to` field is a guess at a module, not a
reference, because the interview writes it before the module exists and the module may never
exist under that name. When a module is enabled, the thread is matched to it by meaning instead.
It ends by deletion once a module that covers it holds a draft developed from it (spec §7).

**The consistency boundary is the store, not the record.** Every change takes the store's single
lock, pulls, rewrites the record and its line in the index, `MEMORY.md`, then commits and pushes.
The index is a projection of every record, and keeping it exact inside the same commit is what
makes each transaction store-wide rather than per-record. With one writer and a few hundred records
that costs nothing. It would be the first thing to revisit if the kernel ever served more than one
writer, because every writer would contend for the one lock.

Records refer to each other by identifier, never by containment: a goal names the values it
`serves`, and each value remains its own record. The crossing kind is not a reference at all. A
`memory/preference` and the `identity/preference` it becomes are the same file; ratification
changes which module governs it, not where it lives, so the record keeps one history (spec §7:
"Model proposes, person ratifies, one file").

**Claims (M2) belong to the goal**, so the goal is their aggregate root. Their results are the
kernel's, not the person's, and they are written by a claim-result operation of their own that
never changes the goal's content or its `updated` stamp (spec §8.1). A plain write would move
`updated`, and because the revision line compares `updated` against `reviewed`, every scheduled
run would then mark its goal "revised since the last review". Keeping results off those two
halves means the goal's content and history stay what the person said, and the revision line
reports only what they changed. Whether results sit in a stamp-like section of the goal or in a
file of their own is decided in M2.

## Module set

|          |                                                                                                                                                                               |
| -------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| root     | the module set (`module.Set`)                                                                                                                                                 |
| identity | the deployment: one set per kernel instance, loaded at start                                                                                                                  |
| inside   | manifests (`module.Manifest`), each with its kinds (`module.Kind`), its mode's bundle (`module.Bundle`) and, from M2, its `intro` and each kind's `lenses` and draft question |
| factory  | `module.Load`, which validates every manifest and then the set as a whole                                                                                                     |
| context  | Schema                                                                                                                                                                        |

The module set is created once, validated whole, and never changed while the kernel runs. That
makes it closer to an immutable value than to an entity: changing the set means restarting the
kernel. It enforces two groups of invariants. Some only the whole set can check, such as the
budgets fitting under the cap together. The rest each manifest checks alone, and `Load` runs those
before accepting any manifest, so a kernel never starts with part of a module set.

The mode and its bundle are value objects. The bundle is the published statement of a mode's
rules, but the kernel's contexts check a module's mode directly instead of reading the bundle;
[`bounded-contexts.md`](bounded-contexts.md) describes that disagreement.

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

Four operations belong to no single aggregate. Each is a function of its inputs and keeps no
state of its own:

| service                 | does                                                                                        |
| ----------------------- | ------------------------------------------------------------------------------------------- |
| `agenda.Compute`        | orders what is due, from records and the module set, each item with its reason and question |
| `block.Renderer.Render` | renders the context block within the cap and the budgets                                    |
| `module.Set.RuleFor`    | names the governing module and kind for a record, which is how the crossing kind works      |
| `server.RenderContext`  | the one path to the block for a caller: forbidden modules, scope, agenda, render            |

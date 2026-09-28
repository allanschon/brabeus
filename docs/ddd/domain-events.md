# Domain events

A domain event is a fact the domain cares about, named in the past tense, that another part of the
system reacts to. Brabeus has no explicit events. Its event log is the store's git history: every
change to a record is one commit with a subject that names what happened, and every other part of
the system learns about a change by reading state on its next request. Terms are the
[glossary's](ubiquitous-language.md).

## When an event is worth making explicit

An event is worth making explicit only when both of these conditions hold:

1. another part of the system has to react to the fact; and
2. that part could not do as well by reading the store, or its git history, the next time it runs.

The second condition is rarely met, because the kernel keeps no state between requests: spec §9
computes the agenda from fields the records already carry, and the context block is rendered fresh
for every request. Whatever needs to know about a change finds it by reading. An event that fails
either condition stays implicit in its commit, where it costs nothing to keep.

## The candidates

| event                                           | recorded today as                                                                                                                   | reacts to it                                                                                                                                           | explicit?                                                                                        |
| ----------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------ |
| record written, a draft included                | a commit, `<module>/<kind>: <name>`                                                                                                 | the index (same commit); the search index (rebuilt from the working copy); the agenda and the block (next request)                                     | no: every reaction reads state                                                                   |
| record reviewed — confirmed, corrected, snoozed | a commit, `review <module>/<kind> <name>: <verdict>`, with the question and verdict in the body, and from M2 the person's answer    | the agenda and the block (next request)                                                                                                                | no: both read the new stamps on their next request                                               |
| record retired                                  | a review commit with verdict `retired`                                                                                              | the agenda and the block, which drop it                                                                                                                | no: both read the `retired` stamp on their next request                                          |
| record deleted                                  | a commit, `delete <module>/<kind> <name>`; before 2026-09-28, `memory: remove <path>`                                               | the index (same commit)                                                                                                                                | no: the only reaction happens inside the same commit                                             |
| records migrated                                | one commit, `migrate: tag <n> records with module and kind`                                                                         | nothing; it happens once                                                                                                                               | no: nothing reacts to it                                                                         |
| module enabled                                  | nothing in the store; the `/healthz` line names the enabled modules and modes                                                       | the write guard, which reads the modes at session start; the interview, which opens a newly enabled module with the threads whose topic it covers (M2) | no: the guard reads state once per session, and the interview finds threads by reading the store |
| block rendered                                  | nothing                                                                                                                             | nothing; a fault is reported in the `context` tool's reply and read by `/health`                                                                       | no: a fault travels with the block it describes                                                  |
| consumer refused                                | nothing; an unresolved caller is logged                                                                                             | nothing                                                                                                                                                | no: nothing reacts to it                                                                         |
| claim checked (M2)                              | not built; spec §8.1 has the kernel's claim-result operation record results for the goal, without touching its content or `updated` | the agenda (a `fail` heads it), `/health` (a `no-evidence` is a fault), the view                                                                       | no: see below                                                                                    |
| work done (`/done`)                             | not built; the done-statement lives in the working repository, not the store                                                        | nothing in Brabeus                                                                                                                                     | no, but see below                                                                                |

## Claim results (M2)

A claim check is the one candidate that carries a result across context boundaries: Intent
produces it, Record stores it, and Ratification acts on it. It still fails the second condition.
The agenda reads results on its next request, which is the start of the next session. That is
exactly when a failed claim needs to surface, so an event would deliver it no sooner than reading
does.

What it has instead of an event is its own operation. Spec §8.1 has results written by the kernel's
claim-result operation, with a commit subject of its own, which never changes the goal's content or
its `updated` stamp. A plain write would have moved `updated`, and the revision line would then have
reported every scheduled run as a revision of the goal (see [`aggregates.md`](aggregates.md)). To
keep the history readable, a result is committed only when a claim's state changes, together with
one record per run saying when claims last ran; the storage itself is decided in M2.

## The done-statement

`/done` depends on nothing in the kernel (spec §14). A done-statement's claims run under the working
repository's tooling, and the evidence they produce — commits, closed tracker items — is exactly what
the `forge` and `tracker` adapters already count. A goal whose claims name those adapters therefore
sees finished work on its next scheduled check, by reading the evidence source, with no event
between `/done` and the kernel. An explicit event would be warranted only if a done-statement's own
claim results, rather than the work they describe, were to become evidence for a goal.

## Commit subjects are a contract

Because the git history is the event log, its commit subjects are the only record of which kind of
change happened. Spec §9's revision line — "target lowered from 3 to 2 on 2026-09-04" — is built by
reading that history: `store.Revision` walks a record's commits back to its last `review …`
subject, diffs the field values at that commit against the file now, and reads the kernel's own
`updated` for the date. So, from M2, the kernel consumes its own commits as events.

The subjects have four shapes: `<module>/<kind>: <name>`, `review <module>/<kind> <name>:
<verdict>`, `delete <module>/<kind> <name>` and `migrate: tag <n> records …`. The delete commit
used to be the odd one out, `memory: remove <path>`, naming a path where the others name a module,
kind and record, and its body named the kernel by its name from before Brabeus. It now takes the
review commit's shape, falling back to the path for a file that predates modules, and its body
reads "Deleted through the kernel at <time>.", as a review's does. The change was made before M2
because nothing reads the subjects yet; once the revision line does, every shape change has to
handle old and new forms alike. A review commit's body belongs to the same contract:
it carries the question and the verdict today, and from M2 the person's answer, which will be the
only record of what they said.

Once a reader depends on these shapes, they are a published language like `module.json`: changing
one breaks whatever reads it, so a change to a commit's shape has to be made together with a change
to its readers.

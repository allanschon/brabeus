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

| event                                           | recorded today as                                                                                                                | reacts to it                                                                                                                                           | explicit?                                                                                        |
| ----------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------ |
| record written, a draft included                | a commit, `<module>/<kind>: <name>`                                                                                              | the index (same commit); the search index (rebuilt from the working copy); the agenda and the block (next request)                                     | no: every reaction reads state                                                                   |
| record reviewed — confirmed, corrected, snoozed | a commit, `review <module>/<kind> <name>: <verdict>`, with the question and verdict in the body, and from M2 the person's answer | the agenda and the block (next request)                                                                                                                | no: both read the new stamps on their next request                                               |
| record retired                                  | a review commit with verdict `retired`                                                                                           | the agenda and the block, which drop it                                                                                                                | no: both read the `retired` stamp on their next request                                          |
| record deleted                                  | a commit, `memory: remove <path>`                                                                                                | the index (same commit)                                                                                                                                | no: the only reaction happens inside the same commit                                             |
| records migrated                                | one commit, `migrate: tag <n> records with module and kind`                                                                      | nothing; it happens once                                                                                                                               | no: nothing reacts to it                                                                         |
| module enabled                                  | nothing in the store; the `/healthz` line names the enabled modules and modes                                                    | the write guard, which reads the modes at session start; the interview, which opens a newly enabled module with the threads whose topic it covers (M2) | no: the guard reads state once per session, and the interview finds threads by reading the store |
| block rendered                                  | nothing                                                                                                                          | nothing; a fault is reported in the `context` tool's reply and read by `/health`                                                                       | no: a fault travels with the block it describes                                                  |
| consumer refused                                | nothing; an unresolved caller is logged                                                                                          | nothing                                                                                                                                                | no: nothing reacts to it                                                                         |
| claim checked (M2)                              | not built; spec §8.1 records results on the goal                                                                                 | the agenda (a `fail` heads it), `/health` (a `no-evidence` is a fault), the view                                                                       | no, but see below                                                                                |
| work done (`/done`)                             | not built; the done-statement lives in the working repository, not the store                                                     | nothing in Brabeus                                                                                                                                     | no, but see below                                                                                |

## Claim results (M2)

A claim check is the one candidate that carries a result across context boundaries: Intent
produces it, Record stores it, and Ratification acts on it. It still fails the second condition.
Results are stored on the goal and the agenda reads them on its next request, which is the start of
the next session. That is exactly when a failed claim needs to surface, so an event would deliver it
no sooner than reading does.

What it needs instead of an event is its own operation. As a plain write, a stored result moves the
goal's `updated` stamp, and the revision line would report every scheduled run as a revision of the
goal (see [`aggregates.md`](aggregates.md)). A kernel operation for recording a claim result, with a
stamp of its own and a commit subject of its own, keeps the result on the goal without making the
goal look edited.

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
reading that history, so from M2 the kernel will consume its own commits as events.

Today the subjects have four shapes: `<module>/<kind>: <name>`, `review <module>/<kind> <name>:
<verdict>`, `memory: remove <path>` and `migrate: tag <n> records …`. The delete commit's body
still names the kernel by its previous name. A review commit's body belongs to the same contract:
it carries the question and the verdict today, and from M2 the person's answer, which will be the
only record of what they said.

Once a reader depends on these shapes, they are a published language like `module.json`: changing
one breaks whatever reads it, so a change to a commit's shape has to be made together with a change
to its readers.

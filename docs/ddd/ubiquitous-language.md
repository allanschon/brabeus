# Ubiquitous language

This glossary lists the terms Brabeus uses, grouped by the bounded context each belongs to
([`bounded-contexts.md`](bounded-contexts.md)). Each term has one meaning inside its context, and
the notes after each table separate a term from the ordinary words it could be mistaken for. **Plain wording** is how the
[plain-language description](../personal-context-system-plain.md) says it, for a reader who uses
the system rather than builds it. **Also called** lists every other name the same concept goes by
in the spec, the code, the tools or the plugin, including names the code still uses where this
glossary has settled on another. A milestone in brackets, such as (M2), names the milestone a
term arrived in; §14 of the specification says which milestones are delivered.

## People and parts

| term       | meaning                                                                                                                        | plain wording  | also called                       |
| ---------- | ------------------------------------------------------------------------------------------------------------------------------ | -------------- | --------------------------------- |
| person     | the human whose record this is                                                                                                 | you            | the owner                         |
| operator   | the human who runs a deployment; in a one-person deployment, the person                                                        | —              | —                                 |
| assistant  | the AI agent in a session, reading the context block and calling the tools                                                     | your assistant | the model, when seen as an author |
| session    | one conversation between the person and the assistant, on one machine                                                          | a conversation | —                                 |
| machine    | a host a session runs on; what machine scope names and what caller resolution returns                                          | a computer     | host                              |
| deployment | one running kernel with its enabled modules, its store and its configuration                                                   | your setup     | instance                          |
| kernel     | the server: sole writer of the store, schema enforcement, retrieval, the agenda and the context block                          | a small server | the server                        |
| plugin     | the assistant-side client: three hooks and a few skills                                                                         | a plugin       | —                                 |
| module     | a directory with a manifest declaring kinds, interview prompts and lenses, a summary template and a budget, run under one mode | a module       | —                                 |

*Kernel* is not an operating-system kernel, and *module* is not a Go module, although this
repository is one.

## The store

| term        | meaning                                                                                                                                                                                                                                                                | plain wording                    | also called                                       |
| ----------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------- | ------------------------------------------------- |
| record      | one fact, stored as one markdown file with frontmatter the kernel composes                                                                                                                                                                                             | a record                         | a memory; `store.Stored`                          |
| the store   | the private git repository that holds every record                                                                                                                                                                                                                     | your record                      | the record; the record repository; `repo: memory` |
| content     | what a caller may say about a record: name, description, module, kind, id, scope, fields and body                                                                                                                                                                      | —                                | `store.Record`                                    |
| stamps      | what only the kernel writes about a record: `updated`, `reviewed`, `retired`, the snooze count and when it was last put off                                                                                                                                                                    | —                                | `store.Meta`                                      |
| kind        | a module-defined type of record, such as `goal`, `value` or `trap`                                                                                                                                                                                                     | —                                | type, before modules                              |
| field       | a value a kind declares, required or optional, set by the caller                                                                                                                                                                                                       | —                                | —                                                 |
| kernel key  | a frontmatter key a caller may not set: `name`, `description`, `module`, `kind`, `id`, `scope`, `updated`, `reviewed`, `retired`, `snoozes`, `snoozed`                                                                                                                          | —                                | reserved field                                    |
| scope       | where a record applies: `global`, `project/<slug>` or `machine/<host>`; every record carries one, `global` unless written with another                                                                                                                                 | —                                | —                                                 |
| index       | `MEMORY.md`, the one-line-per-record list every write maintains                                                                                                                                                                                                        | —                                | —                                                 |
| one writer  | the rule that only the kernel commits to the store                                                                                                                                                                                                                     | only your system can write to it | single writer                                     |
| migration   | the one-time retagging of pre-module records from `type` to `module` and `kind`                                                                                                                                                                                        | —                                | retag                                             |
| legacy type | a record's pre-module `type`, mapped to a kind through `legacy_types`                                                                                                                                                                                                  | —                                | the `type` alias                                  |
| notebook    | the person's own notes repository, which they and the assistant write outside the kernel; the kernel reads and searches it (lexically, or by meaning when `BRABEUS_NOTEBOOK_EMBED` is set) as `repo: notebook` (`repo: projects` still works), never writes it, and refuses it to consumers; the one exception to one repository per kernel instance | your notes                       | the mirror; the projects mirror                   |

*Record* means one file, and the whole repository is *the store*. The spec, and the name of the
`ratified-record` mode, also call the whole repository "the record"; this glossary keeps the two
apart, because a rule about one file and a rule about every file are different rules.

*Memory* is the name of one module, `memory`, and part of the name of one mode, `working-memory`.
The tools also use it for any record, and `MEMORY.md` is the index.

*Kind* is not Kubernetes' kind, and not the ordinary "kind of": the plain-language description's
"two kinds of record" means the two modes.

## Schema

| term               | meaning                                                                                                                                                              | plain wording                 | also called                               |
| ------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------- | ----------------------------------------- |
| mode               | one of the kernel's two closed rule sets — author, read path, freshness, audience default, review — that a module runs under                                         | two kinds of record           | profile; `module.Profile`                 |
| working-memory     | the mode for the assistant's own notes: model-written, searched, never in the context block                                                                          | the assistant's notes         | —                                         |
| ratified-record    | the mode for the person's record: created by the assistant or the person, confirmed only by the person, rendered in the block, interviewed                           | your record                   | the personal record                       |
| bundle             | the fixed rules a mode stands for                                                                                                                                    | —                             | `module.Bundle`                           |
| manifest           | `module.json`, a module's declaration                                                                                                                                | —                             | —                                         |
| core module        | a module whose audience is pinned to `self`: `identity`, `telos`, `health`, `finance`                                                                                | the four modules everyone has | —                                         |
| priority           | a module's order in the block and on the agenda; lower comes first                                                                                                   | —                             | —                                         |
| budget             | a ratified-record module's share of the block, in bytes                                                                                                              | a budget within the block     | `budget_bytes`                            |
| layout             | whether a working-memory module's paths follow `<module>/<kind>/<slug>.md` (`kind`) or the store's own tree (`free`)                                                 | —                             | —                                         |
| scope keys         | the machine and project the plugin fills in automatically when the assistant writes to a module that declares them; in this version only a working-memory module may | —                             | `scope_keys`                              |
| crossing kind      | the one kind that exists in both modes, `preference`: written under `memory`, ratified under `identity`, one file                                                    | —                             | the preference overlap; `module.Crossing` |
| governing module   | the module whose rules decide how a record is interviewed and rendered: its own, or for a crossing record the ratified module it crosses to                          | —                             | `Set.RuleFor`                             |
| onboarding         | a ratified-record module's ordered list of kinds to ask for when none is on file                                                                                     | —                             | —                                         |
| first question     | a kind's question for when nothing of that kind is on file                                                                                                           | —                             | `first`                                   |
| interview question | a kind's question for a record past its freshness; "Is this still right?" when the kind declares none                                                                | —                             | `interview`                               |
| draft question     | a ratified kind's question for a draft; "Is this right as written?" when the kind declares none                                                                      | —                             | `draft`                                   |
| lens               | one of two to four ways into a ratified kind for a conversation, lowering the bar where the first question asks directly                                             | ways that make answering easy | `lenses`                                  |
| intro              | a module's one line, used when the interview turns to it                                                                                                             | —                             | `intro`                                   |
| module set tool    | the read-only tool that serves the enabled modules, their kinds, fields, lenses and intros, withholding any module the caller may not read (M2)                      | —                             | the `modules` tool                        |
| freshness          | how long a ratified record may go unreviewed before it is due, per kind; a kind that declares none is never due by age                                               | shelf life                    | `freshness_days`                          |
| due field          | a date field a kind names so that its records fall due once the date has passed and they have not been reviewed since, such as a decision's revisit date (M2)        | —                             | `due_field`                               |
| timeless           | a working-memory kind exempt from the freshness lint                                                                                                                 | —                             | —                                         |

§7 calls `memory` a core module "in a different sense" from the four above: it is what the kernel
was built for, not what everyone has.

## Ratification

| term               | meaning                                                                                                                                                                                                                                   | plain wording                                  | also called                                        |
| ------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------- | -------------------------------------------------- |
| review             | the kernel operation that records the person's answer to one agenda question; the only path that moves `reviewed`, and accepted only for a record governed by a ratified-record module, which includes a crossing preference (M2)         | answering                                      | `Store.Review`; the `review` tool                  |
| ratified           | having a `reviewed` stamp and not retired                                                                                                                                                                                                 | confirmed                                      | reviewed                                           |
| draft              | a record in a ratified-record module that has never been reviewed; it may be rewritten until confirmed, and renders marked unconfirmed                                                                                                    | a draft                                        | —                                                  |
| unconfirmed marker | the `(unconfirmed)` a draft carries in the block, whatever its kind                                                                                                                                                                       | hasn't been confirmed                          | —                                                  |
| verdict            | the class of the person's answer: `confirmed`, `corrected`, `retired` or `later`                                                                                                                                                          | confirm, correct, retire, later                | `store.Verdict`                                    |
| answer             | the person's reply to a review's question, in their own words, carried into the review commit (M2)                                                                                                                                        | you answered                                   | —                                                  |
| review input       | everything a review is given: the question asked, the verdict, the person's answer, and for a correction the new content                                                                                                                  | —                                              | `store.ReviewInput`                                |
| confirm            | the `confirmed` verdict: the record still holds as written                                                                                                                                                                                | confirm                                        | —                                                  |
| snooze             | a `later` verdict, counted on the record so a deferral stays visible                                                                                                                                                                      | later; putting it off                          | —                                                  |
| retired            | a record the person said no longer applies; kept in history, never rendered or asked about                                                                                                                                                | retire                                         | —                                                  |
| agenda             | the ordered list of what the interview should ask, computed from the store and never stored                                                                                                                                               | —                                              | —                                                  |
| agenda item        | one entry on the agenda, with its question, revision line and snooze count, and for an instruction the text it delivers                                                                                                                                                              | the one thing the system most wants to ask you | `agenda.Item`                                      |
| agenda line        | the top agenda item, rendered as the first line of the block                                                                                                                                                                              | the first line                                 | the first line of every session                    |
| reason             | why an item is on the agenda: `fail` (a claim failed), `behind` (a claim whose pace says it may miss its deadline), `draft` (never reviewed, M2), `stale` (past freshness or past its due field), `onboarding` (nothing of a wanted kind is on file) or `budget` (a module's instructions are over their budget); within each reason, preferences sort last (M2) | —                                              | —                                                  |
| native item        | an agenda item governed by its own module                                                                                                                                                                                                 | —                                              | tier 0                                             |
| crossing item      | an agenda item for a crossing record, governed by another module; as a preference, it sorts with the other preferences, last within its reason (M2)                                                                                       | —                                              | tier 1                                             |
| revision line      | a note that a record changed since it was last reviewed                                                                                                                                                                                   | whether a goal has been quietly lowered        | —                                                  |

*Review* is not code review or an inspection; it is the person answering a question. `reviewed`
moves on `confirmed`, `corrected` and `retired`, and not on `later`: a deferral says nothing about
whether the record holds, so the record stays on the agenda with its snooze count (spec §9).

A *verdict* classifies the person's answer, and the *answer* is what they said. `store.ReviewInput`
in the code is the review input, not the answer; its `Answer` field is. A verdict always comes from the person; a *claim
state* comes from an adapter.

*Draft* has three senses: a never-reviewed ratified record, the agenda reason that names one, and
the manifest key holding the question asked of one. This glossary uses *draft* for the record and
*draft question* for the key. A crossing `memory/preference` the assistant wrote is a draft too
(M2): it is due at once, and because preferences sort last within each reason, it is asked only
when nothing else is due.

## Interview

| term                | meaning                                                                                                                                                                                                                              | plain wording                                   | also called        |
| ------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | ----------------------------------------------- | ------------------ |
| interview           | the conversation that writes the record and keeps it true, built on the agenda and confirming only through review                                                                                                                    | the interview                                   | `/interview`       |
| getting to know you | the interview's way when the top agenda item is onboarding: it walks modules and their onboarding kinds, opening each with its lenses, or its first question when it has none (M2)                                                   | it gets to know you                             | —                  |
| check-in            | the interview's way when the top agenda item is a failed claim, a claim that is behind, a draft, a stale record or a budget item: it opens with that item and follows the conversation (M2)                                                         | a check-in                                      | —                  |
| register            | an `identity` kind: how the person wants to be spoken to, delivered as an instruction to every session and subagent; first in identity's onboarding (M2)                                                                             | how you like to be spoken to                    | —                  |
| preference          | an `identity` kind, and the crossing `memory` kind once confirmed: a standing rule for how the person wants to work, delivered as an instruction to every session and subagent                                                       | a rule you've asked it to follow                | —                  |
| thread              | a `memory` kind: a topic the person raised that no enabled module holds, with a guess at the module it belongs in; found by meaning when a module that covers it is enabled, developed into that module's records, then deleted (M2) | notes it so a later conversation can pick it up | —                  |
| pointer-only thread | a thread whose `belongs_to` names a core module with `audience: self`: it names the topic, and the kernel refuses it a body (M2)                                                                                                     | —                                               | —                  |
| labelled inference  | anything the interviewer adds beyond the person's words — an inference, a suggested date, a strategy — marked as its own                                                                                                             | its own suggestions                             | —                  |
| challenge           | the interviewer's pushback: an entry in the wrong kind, a goal nobody could measure, a contradiction with the record, a statement implying more than it says                                                                         | push back                                       | —                  |
| reflection          | the interview's closing summary of the gap, grouped by value, phrased by the interviewer from what `reflect` returns (M2)                                                                                                            | reflects back                                   | —                  |
| reflect             | the read-only kernel tool that computes the reflection's facts on demand: each value, the goals serving it with their claim states and days since confirmed, and the goals serving no value; nothing is stored (M2)                  | —                                               | the `reflect` tool |

*Getting to know you* and *check-in* are the interview's two ways, not modes: a *mode* is one of
the kernel's two rule sets. A *register* is not a persona: it gives the assistant no name and no
character (spec §2.3).

## Retrieval

| term       | meaning                                                                         | plain wording | also called        |
| ---------- | ------------------------------------------------------------------------------- | ------------- | ------------------ |
| search     | relevance-ranked retrieval, lexical and dense, filtered by scope and audience   | searching     | recall             |
| lexical    | keyword ranking, by BM25                                                        | —             | keyword-only       |
| dense      | meaning-based ranking, by embeddings                                            | —             | meaning-based      |
| hybrid     | lexical and dense results fused                                                 | —             | —                  |
| vocabulary | the store's menu of scopes, modules, kinds and prefixes, returned with a search | —             | `store.Vocabulary` |
| similar    | the duplicate warning a write returns; never a refusal                          | —             | duplicate          |

## Callers and audience

| term                | meaning                                                                                                                                 | plain wording                      | also called                            |
| ------------------- | --------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------- | -------------------------------------- |
| caller              | the machine name the kernel resolved for a request                                                                                      | —                                  | identity; `internal/identity`          |
| caller resolution   | how the kernel decides who is calling                                                                                                   | —                                  | identity (§4.1)                        |
| caller mode         | where caller resolution gets its answer: the private network, or a bearer token                                                         | —                                  | identity mode; `BRABEUS_IDENTITY_MODE` |
| own session         | a caller that is one of the person's own sessions                                                                                       | your own assistant                 | —                                      |
| consumer            | any caller that is not one of the person's own sessions, such as another agent or a dashboard; read-only, and sees only `audience: any` | another program allowed to read it | read-only consumer; untrusted reader   |
| unidentified caller | a request the kernel could not match to a machine or token; refused before any tool runs (M2)                                           | —                                  | —                                      |
| audience            | which callers may read a module: `self` (own sessions only) or `any`                                                                    | the modules you've allowed         | —                                      |
| forbidden modules   | the modules a caller may not see in this session, from audience and, for search, the mode asked for                                     | —                                  | hide-set; `store.Visibility`           |
| credential refusal  | a write matching a credential shape is refused before it reaches git                                                                    | —                                  | the secrets boundary                   |

In this glossary, *identity* means the `identity` module, which holds who the person is. The code's
`internal/identity` package resolves callers, and is listed above under *caller*.

*Scope* is where a record applies, and *audience* is who may read its module. The kernel enforces
the two independently, because scope does not say who may read a record (spec §11).

## The session

| term                | meaning                                                                                                                                                                                                               | plain wording                                | also called                                             |
| ------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------- | ------------------------------------------------------- |
| context block       | the rendered text, at most 2048 bytes, the assistant receives at session start: the agenda line, then each ratified module's summary                                                                                  | a short block of context                     | the block; `context` (tool and route); `internal/block` |
| cap                 | the block's hard limit, 2048 bytes                                                                                                                                                                                    | about two kilobytes                          | —                                                       |
| reservation         | the agenda line's 256-byte share of the cap, outside every budget                                                                                                                                                     | —                                            | —                                                       |
| fault               | a share of the block that could not be rendered as asked, reported rather than hidden                                                                                                                                 | the system says so                           | —                                                       |
| summary template    | a ratified module's `text/template` that renders its share of the block                                                                                                                                               | —                                            | `summary`                                               |
| write guard         | the `PreToolUse` hook that always denies the assistant's writes to the saved copies of the instructions, and denies writes to scratch when a working-memory module is enabled                                                                                                                             | —                                            | routing guard                                           |
| scratch             | the assistant's built-in per-machine memory directory                                                                                                                                                                 | —                                            | auto-memory                                             |
| outbox              | writes queued on the machine while the kernel is unreachable, drained at the next session start                                                                                                                       | —                                            | offline outbox                                          |
| view                | the read-only HTML rendering of the record: a home page and a page per module (M3)                                                                                                                                                                    | a view                                       | —                                                       |
| instructions        | the confirmed, unretired records of the kinds a module marks `instructions`, delivered in full to every session and every subagent beside the block; `identity` marks `preference` and `register`                     | the standing rules for working with you      | `instructions: true`; `internal/instructions`           |
| instructions budget | the size, in bytes, a module's instructions should stay within, declared as `instructions_budget_bytes`; outside the block's cap, and exceeded without cutting anything                                               | how much of your standing rules it can carry | `instructions_budget_bytes`                             |
| budget item         | the agenda item raised when a module's instructions exceed their budget: it names the module, not a record, comes last on the agenda and is never deferred                                                            | a question about trimming your rules         | reason `budget`                                         |
| saved copy          | the instructions a session received at its start, kept by the plugin as `instructions/<session_id>.json` in its data directory for that session's subagents and for a later offline start; written only by the plugin | —                                            | the session's copy                                      |
| source              | an optional field of an instructions kind that holds the interviewer's label for where the wording came from, so the label never enters the body every session receives                                               | —                                            | `source`                                                |

A *fault* is a block that could not render as asked; `no-evidence` is a claim that could not be
checked. They are different failures in different parts of the system.

## Intent (M2)

| term                   | meaning                                                                                                                                  | plain wording                                           | also called        |
| ---------------------- | ---------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------- | ------------------ |
| claim                  | a short, binary statement attached to a goal, naming the evidence that would test it                                                     | a small, testable statement                             | —                  |
| adapter                | a declared evidence source: `tracker`, `forge`, `date` or `manual`                                                                       | a named source of evidence                              | —                  |
| claim state            | what was measured, `pass`, `fail` or `no-evidence`, each with its own timestamp, and what it means today, derived on every read: `pass`, `open`, `behind`, `fail`, `no-evidence` or `unchecked`; `open` and `behind` are never stored | it held, it didn't, it isn't due yet, it is falling behind, or the evidence couldn't be reached | — |
| end-state claim        | a claim that says what will be true by a deadline, its own `by` or its goal's; every claim that is not standing | true by a date | — |
| standing claim         | a claim that says what should hold all the time, with `standing: true`, or a `date` claim; one that does not hold is a contradiction now, and it takes no `by` or `effort` | true all the time | — |
| deadline               | the calendar date an end-state claim must be met by, compared in the kernel's `TZ`, UTC when unset | by when | — |
| effort                 | a yes-or-no claim's `<n>d` estimate of the calendar days its work will take the person; without it the claim gives no early warning | how long it will take | — |
| paced claim            | a counting claim measured against its own target over a window from `since` to the deadline: a `min` of 2 or more, or a manual claim with `of` | a claim with a pace | — |
| open                   | the derived state of an end-state claim not met yet, with its deadline ahead and its work on pace; never on the agenda | not due yet | — |
| behind                 | the derived state of an end-state claim not met yet whose pace says it may miss its deadline: below half its expected count, in whole items rounded down, or no more days left than its `effort` | falling behind | — |
| done-statement         | the four-section document `/done` scaffolds before work starts                                                                           | —                                                       | —                  |
| gap                    | the distance between what the person said matters and what the store shows                                                               | the gap                                                 | —                  |
| serves                 | a goal's link to the values it serves                                                                                                    | —                                                       | —                  |
| claim interval         | how often non-manual claims run: one interval per deployment, daily by default; a `pass` older than two intervals is shown as stale (M2) | on a schedule                                           | —                  |
| claim-result operation | the kernel operation that records a claim's result, adapter or manual, without changing the goal's content or its `updated` stamp (M2)   | —                                                       | —                  |

## Origins

Several terms come from the prior art the spec draws on (§2). The table says where each came from
and what changed on the way.

| term                         | from                               | what changed                                                                            |
| ---------------------------- | ---------------------------------- | --------------------------------------------------------------------------------------- |
| `telos`                      | LifeOS TELOS                       | its sections became kinds; narratives and fixed dimensions were dropped                 |
| `current`, `ideal`           | LifeOS current and ideal state     | the dimensions are the person's own                                                     |
| claim                        | LifeOS Ideal State Criteria        | evidence is named by adapter, not by a probe command                                    |
| interview                    | LifeOS Interview skill             | the kernel decides what is due and what counts as reviewed; the conversation is kept    |
| lens                         | LifeOS Interview skill             | its "three ways in" became a manifest key per kind                                      |
| register                     | LifeOS assistant persona           | the voice was kept as the person's own ratified record; the name and character were not |
| `decision`                   | seandavi/lifeos-template `/decide` | unchanged                                                                               |
| timeless, dated or a pointer | obsidian-second-brain              | became the working-memory freshness lint                                                |
| module                       | LifeOS                             | a unit of schema enabled per deployment, not a capability installed for everyone        |

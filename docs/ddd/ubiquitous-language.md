# Ubiquitous language

The terms Brabeus uses, grouped by the bounded context each belongs to
([`bounded-contexts.md`](bounded-contexts.md)). **Plain wording** is how the
[plain-language description](../personal-context-system-plain.md) says it, for a reader who uses
the system rather than builds it. **Also called** lists every other name the same concept goes by
in the spec, the code, the tools or the plugin.

## People and parts

| term | meaning | plain wording | also called |
|---|---|---|---|
| person | the human whose record this is | you | the owner |
| operator | the human who runs a deployment; in a one-person deployment, the person | — | — |
| assistant | the AI agent in a session, reading the context block and calling the tools | your assistant | the model, when seen as an author |
| session | one conversation between the person and the assistant, on one machine | a conversation | — |
| machine | a host a session runs on; what machine scope names and what caller resolution returns | a computer | host; caller |
| deployment | one running kernel with its enabled modules, its record repository and its configuration | your setup | instance |
| kernel | the server: sole writer of the record, schema enforcement, retrieval, the agenda and the context block | a small server | the server |
| plugin | the assistant-side client: two hooks and a few skills | a plugin | — |
| module | a directory with a manifest declaring kinds, interview prompts, a summary template and a budget, run under one profile | a module | — |

*Kernel* is not an operating-system kernel, and *module* is not a Go module, although this
repository is one.

## The record

| term | meaning | plain wording | also called |
|---|---|---|---|
| record | one fact, stored as one markdown file with frontmatter the kernel composes | a record | a memory, in four of the seven tool descriptions; `store.Stored` |
| the record | the whole private repository of records | your record | the store; the record repository |
| kind | a module-defined type of record, such as `goal`, `value` or `trap` | — | type, before modules |
| field | a value a kind declares, required or optional, set by the caller | — | — |
| kernel key | a frontmatter key only the kernel writes: `name`, `description`, `module`, `kind`, `id`, `scope`, `updated`, `reviewed`, `retired`, `snoozes` | — | reserved field; `store.Meta` |
| stamp | a kernel-written RFC 3339 time: `updated` on every write; `reviewed` and `retired` only on review | — | — |
| scope | where a record applies: `global`, `project/<slug>` or `machine/<host>` | — | — |
| index | `MEMORY.md`, the one-line-per-record list every write maintains | — | — |
| one writer | the rule that only the kernel commits to the record repository | only your system can write to it | single writer |
| migration | the one-time retagging of pre-module records from `type` to `module` and `kind` | — | retag |
| legacy type | a record's pre-module `type`, mapped to a kind through `legacy_types` | — | the `type` alias |
| mirror | a second, read-only repository the kernel can list, read and search as `repo: projects`; refused to consumers | — | the projects mirror |

*Record* carries two meanings, told apart only by the article: one file, and the whole
repository. `ratified-record` uses the second. In code the word is narrower again: `store.Record`
is only the part of a file a caller may set, `store.Meta` is the kernel's part, and `store.Stored`
is both with the path.

*Memory* carries five: the `memory` module; the `working-memory` profile; any record, in the
descriptions of `list`, `read`, `search` and `delete`; the record repository, as the `repo: memory`
argument; and the index file, `MEMORY.md`.

*Kind* is not Kubernetes' kind, and not the ordinary "kind of": the plain-language description's
"two kinds of record" means the two profiles.

## Schema

| term | meaning | plain wording | also called |
|---|---|---|---|
| profile | one of the kernel's two closed rule sets — author, read path, freshness, audience default, review — that a module runs under | two kinds of record | bundle |
| working-memory | the profile for the assistant's own notes: model-written, searched, never in the context block | the assistant's notes | — |
| ratified-record | the profile for the person's record: created by the assistant or the person, confirmed only by the person, rendered in the block, interviewed | your record | the personal record |
| bundle | the fixed rules a profile stands for | — | `module.Bundle` |
| manifest | `module.json`, a module's declaration | — | — |
| core module | a module whose audience is pinned to `self`: `identity`, `telos`, `health`, `finance` | the four modules everyone has | — |
| priority | a module's order in the block and on the agenda; lower comes first | — | — |
| budget | a ratified-record module's share of the block, in bytes | a budget within the block | `budget_bytes` |
| layout | whether a working-memory module's paths follow `<module>/<kind>/<slug>.md` (`kind`) or the store's own tree (`free`) | — | — |
| crossing kind | the one kind that exists in both profiles, `preference`: written under `memory`, ratified under `identity`, one file | — | the preference overlap; `module.Crossing` |
| governing module | the module whose rules decide how a record is interviewed and rendered: its own, or for a crossing record the ratified module it crosses to | — | `Set.RuleFor` |
| onboarding | a ratified-record module's ordered list of kinds to ask for when none is on file | — | — |
| first question | a kind's question for when nothing of that kind is on file | — | `first` |
| freshness | how long a ratified record may go unreviewed before it is due, per kind | shelf life | `freshness_days` |
| timeless | a working-memory kind exempt from the freshness lint | — | — |

*Profile* is not the person's profile. §7 calls `memory` a core module "in a different sense"
from the four above: it is what the kernel was built for, not what everyone has.

## Ratification

| term | meaning | plain wording | also called |
|---|---|---|---|
| review | the kernel operation that records the person's answer to one agenda question; the only path that moves `reviewed` | confirming | `Store.Review`; the `review` tool |
| ratified | having a `reviewed` stamp and not retired | confirmed | confirmed; reviewed |
| verdict | the class of the person's answer: `confirmed`, `corrected`, `retired` or `later` | confirm, correct, retire, later | answer |
| snooze | a `later` verdict, counted on the record so a deferral stays visible | later; putting it off | — |
| retired | a record the person said no longer applies; kept in history, never rendered or asked about | retire | — |
| agenda | the ordered list of what the interview should ask, computed from the record and never stored | — | — |
| agenda item | one entry on the agenda, with its question, revision line and snooze count | the one thing the system most wants to ask you | `agenda.Item` |
| agenda line | the top agenda item, rendered as the first line of the block | the first line | the first line of every session |
| reason | why an item is on the agenda: `fail` (a claim failed), `stale` (past freshness) or `empty` (an onboarding question) | — | — |
| native item | an agenda item governed by its own module; native items sort before crossing items | — | tier 0 |
| crossing item | an agenda item for a crossing record, governed by another module | — | tier 1 |
| revision line | a note that a record changed since it was last reviewed | whether a goal has been quietly lowered | — |
| interview | the loop that asks agenda items one at a time and records each answer through review | the interview | `/interview` |
| reflection | the interview's closing summary of the gap, grouped by value (M2) | reflects back | — |

*Review* is not code review; it is the person answering a question. `reviewed` moves on
`confirmed`, `corrected` and `retired`, and not on `later`, so *review*, *ratify* and *confirm*
are not interchangeable. A *verdict* is the person's answer; a *claim state* is an adapter's.

## Retrieval

| term | meaning | plain wording | also called |
|---|---|---|---|
| search | relevance-ranked retrieval, lexical and dense, filtered by scope and audience | searching | recall |
| lexical | keyword ranking, by BM25 | — | keyword-only |
| dense | meaning-based ranking, by embeddings | — | meaning-based |
| hybrid | lexical and dense results fused | — | — |
| vocabulary | the store's menu of scopes, modules, kinds and prefixes, returned with a search | — | `store.Vocabulary` |
| similar | the duplicate warning a write returns; never a refusal | — | duplicate |

## Callers and audience

| term | meaning | plain wording | also called |
|---|---|---|---|
| caller | the machine name the kernel resolved for a request | — | identity |
| identity mode | how the kernel resolves the caller: from the private network, or from a bearer token | — | `BRABEUS_IDENTITY_MODE` |
| own session | a caller that is one of the person's own sessions | your own assistant | — |
| consumer | any caller that is not one of the person's own sessions, such as another agent or a dashboard; read-only, and sees only `audience: any` | another program allowed to read it | read-only consumer; untrusted reader |
| audience | which callers may read a module: `self` (own sessions only) or `any` | the modules you've allowed | — |
| hide-set | the modules a caller may not see in this session, from audience and, for search, the profile asked for | — | `store.Visibility` |
| credential refusal | a write matching a credential shape is refused before it reaches git | — | the secrets boundary |

*Identity* in §4.1 and in `internal/identity` means caller resolution. It is unrelated to the
`identity` module, which is who the person is. *Scope* is where a record applies; *audience* is
who may read its module; the two are enforced independently.

## The session

| term | meaning | plain wording | also called |
|---|---|---|---|
| context block | the rendered text, at most 2048 bytes, the assistant receives at session start: the agenda line, then each ratified module's summary | a short block of context | the block; `context` (tool and route); `internal/block` |
| cap | the block's hard limit, 2048 bytes | about two kilobytes | — |
| reservation | the agenda line's 256-byte share of the cap, outside every budget | — | — |
| fault | a share of the block that could not be rendered as asked, reported rather than hidden | the system says so | — |
| summary template | a ratified module's `text/template` that renders its share of the block | — | `summary` |
| guard | the `PreToolUse` hook that denies writes to the assistant's built-in per-machine memory | — | write guard; routing guard |
| scratch | the assistant's built-in per-machine memory directory | — | auto-memory |
| outbox | writes queued on the machine while the kernel is unreachable, drained at the next session start | — | offline outbox |
| view | the read-only HTML rendering of the record (M3) | a view | — |

A *fault* is a block that could not render as asked; `no-evidence` is a claim that could not be
checked. They are different failures in different parts of the system.

## Intent (M2)

| term | meaning | plain wording | also called |
|---|---|---|---|
| claim | a short, binary statement attached to a goal, naming the evidence that would test it | a small, testable statement | — |
| adapter | a declared evidence source: `tracker`, `forge`, `date` or `manual` | a named source of evidence | — |
| claim state | `pass`, `fail` or `no-evidence`, each with its own timestamp | it held, it didn't, or the evidence couldn't be reached | three-state result |
| done-statement | the four-section document `/done` scaffolds before work starts | — | — |
| gap | the distance between what the person said matters and what the record shows | the gap | — |
| serves | a goal's link to the values it serves | — | — |

## Origins

| term | from | what changed |
|---|---|---|
| `telos` | LifeOS TELOS | its sections became kinds; narratives and fixed dimensions were dropped |
| `current`, `ideal` | LifeOS current and ideal state | the dimensions are the person's own |
| claim | LifeOS Ideal State Criteria | evidence is named by adapter, not by a probe command |
| interview | LifeOS Interview skill | the kernel chooses the question |
| `decision` | seandavi/lifeos-template `/decide` | unchanged |
| timeless, dated or a pointer | obsidian-second-brain | became the working-memory freshness lint |
| module | LifeOS | a unit of schema enabled per deployment, not a capability installed for everyone |

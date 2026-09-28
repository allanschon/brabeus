# Bounded contexts

Brabeus is one kernel, one plugin and a set of modules, but its model is not one model. A
*record* means a file with frontmatter to the store, a scored document to retrieval, and a
question waiting for an answer to ratification. Each context below is a boundary inside which
the terms in [`ubiquitous-language.md`](ubiquitous-language.md) have one meaning. A context is a
boundary of language and consistency, not a package; several span more than one package, and
the places where they do are noted.

## Subdomains

| class      | contexts                                                      | why                                                                                                                                                                                                                                                              |
| ---------- | ------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| core       | Ratification, Schema, Context block, Intent, Interview        | what spec §2.4 names as distinctive: authorship enforced by the server, modes a manifest cannot edit, the 2 KB cap as a constraint, claims verified against evidence; and the conversation that writes the record, without which there is nothing to ratify (§9) |
| supporting | Record, Callers and audience, Assistant integration, Notebook | built for Brabeus because nothing off the shelf fits, but not what makes it Brabeus                                                                                                                                                                              |
| generic    | Retrieval, Deployment                                         | BM25, embeddings, container wiring: well-solved problems                                                                                                                                                                                                         |

## The contexts

### Record

Owns records as files: composing frontmatter from content and stamps, the index, scope
validation, credential refusal, the one-writer path of pull, write, commit and push, and the
migration.

Lives in `internal/store` (`store.go`, `record.go`, `commit.go`, `migrate.go`, `credential.go`,
`path.go`, `index.go`) and `internal/scope`.

The review operation is a method on the store (`Store.Review`) rather than part of Ratification's
code, and belongs there: a verdict rewrites a record's stamps, and every change to a record must
happen under the store's single-writer lock, after a pull and before a push. Ratification decides
what to ask and what an answer means; Record applies the answer atomically.

Relationships:

- **Git host** (external, upstream). Record conforms: git is the ledger, a commit's author is the
  calling machine, and a review's question and verdict are its commit message.
- **Schema** (upstream). Record conforms: every write is validated against the governing kind
  and the module's layout.
- **Retrieval** (upstream). Record conforms, including for the frontmatter format itself; see
  *Where the code and the contexts disagree*.
- **Callers and audience**. `internal/scope` is a shared kernel between the two: Record validates
  scopes with it, caller resolution normalises host names with it, and the server filters reads
  with it. It holds value-level functions only.

### Schema

Owns modes, modules, kinds, fields, budgets, the core-module list, the crossing kind and the
governing-module rule.

Lives in `internal/module` and the `modules/` directory.

Relationships:

- **Every other kernel context** (downstream). Each conforms to Schema's model directly, through
  `module.Set`.
- **Module authors**. `module.json` is a published language: a closed set of keys, refused on
  anything unknown.
- **Interview** (downstream, M2). The read-only `modules` tool is an open host service for the
  module set. Its `lenses`, `draft` and `intro` keys are validated by the kernel and read only by
  the Interview; how to ask belongs to the module (§6).

A mode's rules are published as a `Bundle`, but the downstream contexts do not read it; each
compares a module's mode against a constant and applies the rule itself. See *Where the code and
the contexts disagree*.

### Ratification

Owns what is due and what counts as reviewed: the agenda and its ordering, freshness, drafts,
reasons, native and crossing items, verdict semantics, the revision line and snoozes. It does not
own the conversation that asks; spec §9 puts that outside the kernel, in the Interview.

Lives in `internal/agenda` (the agenda) and `internal/store/review.go` (verdicts applied).

The agenda is a read model: computed from the store and the module set on every request, never
stored.

Relationships:

- **Record** (upstream). Conforms: the agenda reads `store.Stored` records as they are.
- **Schema** (upstream). Conforms: freshness, prompts, onboarding and the governing module all
  come from the module set.
- **Context block** (downstream). The top agenda item becomes the agenda line.
- **Interview** (downstream). Reads the agenda, each item with its reason and question, and
  returns answers through `review`.
- **Intent** (upstream, M2). Failed claims will head the agenda.

### Retrieval

Owns ranking: lexical (BM25), dense (embeddings), their fusion, snippets, and the duplicate check.

Lives in `internal/retrieval`, and in `Store.Search` and `Store.SimilarTo`, which apply scope and
audience before ranking.

Relationships:

- **Embedding sidecar** (external, upstream). Behind an anti-corruption layer: the `Embedder`
  interface, with a caching decorator, is the only thing that knows a model is being called.
- **Record** (downstream). The store builds and owns the index; retrieval supplies the ranking.

### Callers and audience

Owns who is calling (caller resolution), the difference between an own session and a consumer,
audience, and the forbidden modules for a caller.

Has no package of its own. Caller resolution is `internal/identity`. The core-module list, which
fixes audience for four modules, is in `internal/module`. The forbidden-modules type is
`store.Visibility`. The policy that joins them — `Caller`, `ParseConsumers`, `audienceFor`,
`hiddenFor` — is in `internal/server`.

Relationships:

- **The private network and bearer tokens** (external, upstream). Behind an anti-corruption
  layer: the `Identity` interface, with one implementation per caller mode, returns a machine
  name and nothing else.
- **Record**. Shared kernel through `internal/scope`.
- **Schema** (upstream). Conforms: audience is a manifest value, and core modules are pinned by
  Schema.
- **Record, Retrieval, Context block** (downstream). Each receives the forbidden modules and
  withholds them.

### Context block

Owns the rendered block: the cap, the reservation, per-module budgets, summary templates, and
faults.

Lives in `internal/block`, and in `server.RenderContext`, which the `context` tool and
`GET /context` share.

Relationships:

- **Record, Schema, Ratification** (upstream). Conforms to all three.
- **Callers and audience** (upstream). A forbidden module contributes nothing to the block, not
  even its name.
- **Assistant integration** (downstream). The block is served as plain text at `GET /context`
  and as the `context` tool: an open host service whose published language is the block's text.

### Assistant integration

Owns what happens on the person's machine around the session: injecting the block at session
start, the write guard, the outbox and its drain, the `health` skill, and the MCP registration.
The `interview` skill ships in the same plugin but belongs to the Interview.

Lives in `plugins/brabeus`.

Relationships:

- **Claude Code** (external, upstream). The plugin is the anti-corruption layer between
  Claude Code's hooks and the kernel: it turns a session start into a block fetch, a write to
  scratch into a denial, and an unreachable kernel into queued outbox files.
- **The kernel** (upstream). Conforms to the MCP tools and to the text of `/context` and
  `/healthz`. The write guard decides from the `profiles=` field of the `/healthz` line, which is
  unversioned text rather than a published contract.

### Interview

Owns the conversation that writes and keeps the record: its two ways — getting to know you and
the check-in — lenses, intros, drafts in the making, threads, the person's register, labelled
inferences, challenges, and reflection by value.

Lives in the plugin's `interview` skill. As built (M1) the skill reads the agenda's question aloud
and files one answer per record; the conversation is M2 (§14).

Its rules are the model's to follow, not the kernel's to enforce: a cue addressed to the model is
advisory (§3.3). What it may change is limited by the contexts it calls. It writes drafts through
Record, which validates them against Schema; it confirms only through `review`, which Ratification
owns; and the agenda, not the conversation, decides what is due.

Relationships:

- **Ratification** (upstream). Conforms to the agenda, its reasons and its questions; returns
  verdicts and the person's answer through `review`.
- **Schema** (upstream). Conforms to the module set, read through the `modules` tool.
- **Record** (upstream). Writes and rewrites drafts and threads through the `write` tool, and
  deletes a thread once a module that covers it holds a draft from it (§7).
- **Context block** (upstream). The person's register renders in the block like any ratified
  record, which is how it reaches every session, not just the interview.
- **Claude Code** (external). Runs as a skill in the person's session.

### Deployment

Owns configuration and wiring: environment variables, the sync interval, the migration flag, the
container image.

Lives in `cmd/brabeus` and the `Dockerfile`. It depends on every kernel context and nothing
depends on it.

### Intent (M2)

Owns claims, adapters, claim states and the schedule that runs them. Not built.

Will live in `internal/claims`, beside `agenda` and `block`. Evidence sources sit behind adapters
by design (§8.1), an anti-corruption layer for each backend, and results reach the store through
Record's write path, so there is still one writer.

### Notebook

Owns nothing the kernel writes. The notebook is the person's own notes repository — notes they
write, notes the assistant writes there at their request, material they keep — which the kernel
can list, read and search, as `repo: projects`, and never writes.

Lives in `cmd/brabeus` (a second `store.Store`, configured by `BRABEUS_MIRROR_*`) and
`server.pickRepo`. It has no modules and no scopes, and no embedder, so its search is lexical
only; whether it gains the dense leg is decided in M2. It bypasses Schema and Callers and audience
entirely, and consumers are refused it outright for that reason (§11).

The notebook is upstream: it has its own writers and its own conventions, and the kernel conforms
to whatever it finds, reusing Record's reading code and none of Record's rules. Spec §5 makes it
the one exception to one repository per kernel instance, and keeps the one-writer rule by never
writing it.

## The context map

Arrows point from upstream to downstream. Labels name the relationship.

```mermaid
flowchart LR
    githost["Git host"]
    embed["Embedding sidecar"]
    net["Private network,<br/>bearer tokens"]
    cc["Claude Code"]
    evidence["Evidence sources<br/><i>tracker, forge</i>"]

    schema["<b>Schema</b><br/><i>core</i>"]
    record["<b>Record</b><br/><i>supporting</i>"]
    retrieval["<b>Retrieval</b><br/><i>generic</i>"]
    callers["<b>Callers and audience</b><br/><i>supporting</i>"]
    ratification["<b>Ratification</b><br/><i>core</i>"]
    blockctx["<b>Context block</b><br/><i>core</i>"]
    plugin["<b>Assistant integration</b><br/><i>supporting</i>"]
    intent["<b>Intent</b><br/><i>core, M2</i>"]
    notebook["<b>Notebook</b><br/><i>supporting</i>"]
    interview["<b>Interview</b><br/><i>core</i>"]

    githost -- "conformist" --> record
    embed -- "ACL: Embedder" --> retrieval
    net -- "ACL: Identity" --> callers
    evidence -. "ACL: adapters" .-> intent

    schema -- "published language:<br/>module.json" --> record
    schema -- "conformist" --> ratification
    schema -- "conformist" --> blockctx
    schema -- "conformist" --> callers
    retrieval -- "conformist" --> record
    record ---|"shared kernel: scope"| callers
    record -- "conformist" --> ratification
    record -- "conformist" --> blockctx
    notebook -- "conformist: reading code only" --> record
    callers -- "forbidden modules" --> blockctx
    callers -- "forbidden modules" --> retrieval
    ratification -- "agenda line" --> blockctx
    intent -. "failed claims" .-> ratification
    blockctx -- "OHS: /context, context tool" --> plugin
    plugin -- "ACL" --> cc
    ratification -- "agenda; review" --> interview
    schema -. "OHS: modules tool" .-> interview
    record -- "OHS: MCP write tools" --> interview
    interview -- "runs as a skill in" --> cc

    classDef system fill:#1168bd,stroke:#0b4884,color:#fff
    classDef container fill:#438dd5,stroke:#2e6295,color:#fff
    classDef ext fill:#999,stroke:#6b6b6b,color:#fff,stroke-dasharray:5 5
    class schema,ratification,blockctx,intent,interview system
    class record,retrieval,callers,plugin,notebook container
    class githost,embed,net,cc,evidence ext
```

Core contexts are dark, supporting and generic ones lighter, externals grey and dashed. Dashed
arrows are M2; the Interview is drawn solid because the M1 skill already reads the agenda and
calls `review`, though its conversation is M2.

## Where the code and the contexts disagree

The package imports follow the layering the contexts need: `retrieval`, `scope` and `module`
import nothing internal, and only `cmd/brabeus` imports `server`. The disagreements are about
ownership, not direction.

- **Retrieval owns the frontmatter format.** `store.ParseRecord` delegates to
  `retrieval.ParseFrontmatter`, because retrieval indexes files and cannot import the store. The
  import points the right way; the ownership does not. The file format is Record's, and
  Retrieval should be handed parsed documents or share a leaf package that owns the format.
- **Callers and audience has no home.** Its model is split across `module`, `store` and `server`,
  and the forbidden-modules type also carries the search-mode narrowing, which is Retrieval's
  concern and which a caller can widen by asking. A module withheld by audience is forbidden; one
  withheld because search defaults to working-memory is not. They are one type today.
- **Schema's published rules are not the enforced ones.** `module.Bundle` states each mode's
  rules, but `ModelWrites`, `Rendered`, `SearchedDefault` and `ReviewRequired` are read nowhere
  outside tests. `block`, `server` and `store` each compare against `module.RatifiedRecord` or
  `module.WorkingMemory` instead, so a mode's meaning lives in four packages rather than one.
- **The Interview ships inside Assistant integration.** One plugin carries both contexts, so the
  hooks and the conversation are released together. The boundary is between skills, not packages.

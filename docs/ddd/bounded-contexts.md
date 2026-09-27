# Bounded contexts

Brabeus is one kernel, one plugin and a set of modules, but its model is not one model. A
*record* means a file with frontmatter to the store, a scored document to retrieval, and a
question waiting for an answer to ratification. Each context below is a boundary inside which
the terms in [`ubiquitous-language.md`](ubiquitous-language.md) have one meaning. A context is a
boundary of language and consistency, not a package; several span more than one package, and
the places where they do are noted.

## Subdomains

| class      | contexts                                                      | why                                                                                                                                                                  |
| ---------- | ------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| core       | Ratification, Schema, Context block, Intent                   | what spec §2.4 names as distinctive: authorship enforced by the server, modes a manifest cannot edit, the 2 KB cap as a constraint, claims verified against evidence |
| supporting | Record, Callers and audience, Assistant integration, Notebook | built for Brabeus because nothing off the shelf fits, but not what makes it Brabeus                                                                                  |
| generic    | Retrieval, Deployment                                         | BM25, embeddings, container wiring: well-solved problems                                                                                                             |

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

A mode's rules are published as a `Bundle`, but the downstream contexts do not read it; each
compares a module's mode against a constant and applies the rule itself. See *Where the code and
the contexts disagree*.

### Ratification

Owns the agenda and its ordering, freshness, reasons, native and crossing items, verdict
semantics, the revision line, snoozes, and the interview loop.

Lives in `internal/agenda` (the agenda), `internal/store/review.go` (verdicts applied), and the
plugin's `interview` skill (the loop).

The agenda is a read model: computed from the store and the module set on every request, never
stored.

Relationships:

- **Record** (upstream). Conforms: the agenda reads `store.Stored` records as they are.
- **Schema** (upstream). Conforms: freshness, prompts, onboarding and the governing module all
  come from the module set.
- **Context block** (downstream). The top agenda item becomes the agenda line.
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

Owns what happens on the person's machine: injecting the block at session start, the write
guard, the outbox and its drain, the `health` and `interview` skills, and the MCP registration.

Lives in `plugins/brabeus`.

Relationships:

- **Claude Code** (external, upstream). The plugin is the anti-corruption layer between
  Claude Code's hooks and the kernel: it turns a session start into a block fetch, a write to
  scratch into a denial, and an unreachable kernel into queued outbox files.
- **The kernel** (upstream). Conforms to the MCP tools and to the text of `/context` and
  `/healthz`. The write guard decides from the `profiles=` field of the `/healthz` line, which is
  unversioned text rather than a published contract.

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
only. It bypasses Schema and Callers and audience entirely, and consumers are refused it outright
for that reason.

The notebook is upstream: it has its own writers and its own conventions, and the kernel conforms
to whatever it finds, reusing Record's reading code and none of Record's rules. No section of the
spec describes it, and it sits against §5: "One repository per kernel instance, in this version."

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

    classDef system fill:#1168bd,stroke:#0b4884,color:#fff
    classDef container fill:#438dd5,stroke:#2e6295,color:#fff
    classDef ext fill:#999,stroke:#6b6b6b,color:#fff,stroke-dasharray:5 5
    class schema,ratification,blockctx,intent system
    class record,retrieval,callers,plugin,notebook container
    class githost,embed,net,cc,evidence ext
```

Core contexts are dark, supporting and generic ones lighter, externals grey and dashed. Dashed
arrows are M2.

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
- **The notebook is a second repository in one kernel.** §5 and §3.8 put one repository behind each
  kernel instance and a second trust domain behind a second instance. The notebook is read-only, so
  it does not break the one-writer rule, but it is a second corpus with no schema and no audience
  of its own inside the same kernel.

# A personal AI context system — v1.9 specification

**Working name: Brabeus.** See §16.

**Status: v1.9. M0, M1 and M2 are built, and v1.9's claim dates; see §14.**

This document describes a system built on the kernel this repository already contains: a
private store with hybrid retrieval, a Claude Code plugin, and a Dockerfile that builds the
kernel. How that image is deployed and updated is each deployment's own concern. The kernel has no
opinions about content. Everything it stores belongs to a module, and every module runs under one
of a small set of profiles the kernel defines (§1.1).

C4 diagrams of the system at v1.8 — context, containers, kernel components, and the interview
as a sequence — are in [`personal-context-system-c4.md`](personal-context-system-c4.md). A
plain-language description for someone who might use it rather than build it is
[`personal-context-system-plain.md`](personal-context-system-plain.md).
The system in the terms of domain-driven design — its ubiquitous language, bounded contexts,
aggregates, invariants and domain events — is in [`ddd/`](ddd/README.md).

This document specifies the system. It does not describe any particular deployment. One
deployment is used as the acceptance test in §13 and is described only in the terms the system
itself uses; the operator's own notes on mapping their environment onto it live elsewhere.

## 1. What this is

A **personal AI context system** is a private, durable record of who a person is, what they
value, what they are trying to do and how it is going — held in a form an AI assistant can read
at the start of every session, add to during it, and be held to account against.

The design goal in one sentence:

> An assistant that knows the person's stated goals and values, checks the record against
> evidence, and reflects the gap back — without the person having to remember to maintain it.

The last clause is the axiom the whole design answers to. A mechanism that relies on the person
noticing, doing or remembering something is rejected or given an automatic backstop.

### 1.1 Two profiles, one kernel

The server this is built on already does a different job well: it is a **working memory** —
the durable notes of an assistant, written by the model as it goes, searched when needed,
replacing the assistant's per-machine scratch files. A store of that kind fills with traps,
pointers and project notes: how to reach a host, which field is not on which screen, what a
tool does silently. Almost none of it is about the person.

The personal record is not that. The two differ on every axis that matters, which is why the
prior art keeps them in separate trees (`MEMORY/` and `USER/`), and why forcing them into one
shape fails. But the axes are not independent — a
model-written record pushed into every session is the model's inferences presented as the
person's; a person-ratified record that is only searched is a record nobody sees — so they are
not declared one by one. **The kernel defines a small closed set of profiles, each a fixed bundle
of the axes, and every module runs under one:**

| | `working-memory` | `ratified-record` |
|---|---|---|
| author | the model, freely, mid-session | the person, through `review` (§9) |
| read | pull: searched on demand, never in the block | push: rendered into the 2 KB block, agenda first (§10); searched only when asked |
| freshness | a record is timeless, dated, or a pointer — linted by `/health`, never interviewed | `reviewed` against `freshness_days` |
| audience default | `self` | `self` |
| `review` | none, except the crossing `preference`, which a review moves under `identity` (§7) | required — `write` may create, only `review` confirms |
| keying | module, kind, and scope keys the plugin resolves (machine, project) | module and kind; a scope only when written with one (§5) |
| interview, claims, agenda | no | yes |

A working-memory module may declare `audience: any`; a core module may not (§6). `review` is how
the person confirms their record, so it accepts only a record whose governing module is
`ratified-record`; a working-memory record has no confirmed state for it to set.

A module declares its kinds, fields, layout, thresholds, adapters and budget (§6), and picks a
profile. It cannot edit the profile. A third profile — a record several people ratify, a memory
shared across a group — is added in the kernel, reviewed, when someone needs it; it is never
declared in a manifest (§15).

The general memory is therefore a module, `memory`, under `working-memory`, shipped with default
kinds (§7). The four core modules of this system run under `ratified-record`, and cannot be moved
to another profile. So the general memory and the personal record are not two products: which of
the two a module is comes down to one word in its manifest, and the kernel enforces what that word
means.

The kernel is the hard part, and both profiles share all of it: the git store with one writer,
frontmatter composed by the server, hybrid retrieval, scope on read, the secrets boundary, the
plugin's outbox and the deploy path. It has no product semantics of its own.

There is one deliberate overlap, and the profiles make it exact: a `preference` the model writes
under `working-memory` and the person confirms under `identity` is the same file, with `reviewed`
set (§7). The model proposes, and the person ratifies.

## 2. Prior art: LifeOS, and what it teaches

[LifeOS](https://github.com/danielmiessler/LifeOS) (Daniel Miessler, formerly *Personal AI
Infrastructure*) is the most developed public attempt at this. It was installed, containerised
and read for this specification — the constitution, the Algorithm, the ISA and Interview skills,
the TELOS templates and the installed hooks and settings. Three of its ideas are the reason this
system exists; its harness is the reason this system is not LifeOS.

### 2.1 What is valuable

- **TELOS** — a written, sectioned statement of mission, goals, problems, challenges, strategies,
  beliefs and narratives, read at every session start. The insight is not the sections; it is
  that *direction is the bottleneck, not execution*, so the person's intent has to be on file
  before any task begins. ([TELOS](https://danielmiessler.com/telos),
  [Personal AI Infrastructure](https://danielmiessler.com/blog/personal-ai-infrastructure))
- **Current state versus ideal state.** Each life dimension has an honest baseline and a
  described ideal, and the gap between them is the signal. LifeOS's own template says it best:
  an ideal-state file scores on articulation, so "a beautifully-written ideal you are nowhere
  near living still scores 100"; writing the current state is what makes the number honest.
- **Ideal State Criteria.** "Done" is written down before work starts as short, binary claims,
  each naming the probe that would prove it false, so "the spec IS the test suite".
  ([Ideal State Articulation](https://danielmiessler.com/blog/ai-ideal-state-articulation),
  [From Prompt Engineering to Intent Engineering](https://danielmiessler.com/blog/intent-engineering))
- **The interview rule.** LifeOS's Interview skill forbids a blank "what's your mission?" when
  a TELOS is on file: read what is there, confront it with observed data, open with the sharpest
  claim-versus-evidence contradiction, ask "still right?", and let the person ratify corrections.
  This is the single best piece of the design.

Independent users converge on the same three, and on less machinery around them: a markdown
vault, a review cadence and a feedback layer, with the assistant as "a critical partner, not
helpful completer" ([seandavi/lifeos-template](https://github.com/seandavi/lifeos-template));
or no system at all, just observation and daily logging until preferences accrete
([Hilary Gridley](https://www.lennysnewsletter.com/p/how-to-turn-claude-code-into-your)).

### 2.2 What makes it unwieldy

Measured on v7.40.4, installed 2026-09-24:

- **The harness.** By the install's own count, 74 hook commands across 11 events (60 scripts,
  some wired to several events) and 57 skills; a 213-entry permission allowlist, `defaultMode: "auto"`, and an `autoMode` block telling the permission classifier to
  auto-approve edits to `settings.json`, the hooks and the system prompt. The constitution is
  25 KB. The ISA format has seventeen sections. The Algorithm doctrine has a changelog longer
  than most specifications.
- **Self-modifying configuration that fails silently.** The update path merges hooks only, so
  settings hardening never reaches an existing install
  ([#1860](https://github.com/danielmiessler/LifeOS/issues/1860)); a reordered array is silently
  pinned and later additions are dropped
  ([#2162](https://github.com/danielmiessler/LifeOS/issues/2162)); `lifeos -m <shortcut>`
  replaces `~/.claude/.mcp.json` with an empty object, no backup, exit 0
  ([#2156](https://github.com/danielmiessler/LifeOS/issues/2156)); the documented
  system/user settings split is never established by the installer
  ([#2086](https://github.com/danielmiessler/LifeOS/issues/2086)).
- **The security model is the model.** The stated posture is that the constitutional rule is
  the boundary and regex layers are not worth having
  ([Security — Minimal v2](https://docs.ourlifeos.ai/Security__README/)). The safety hook then
  auto-allows MCP calls the operator had listed under `permissions.ask`
  ([#2087](https://github.com/danielmiessler/LifeOS/issues/2087)), and the shipped `auto` mode
  steers writes through Bash, silencing four of its own PostToolUse hooks
  ([#2079](https://github.com/danielmiessler/LifeOS/issues/2079)). A Siri endpoint takes a
  prompt from a POST body and hands it to a full agent session behind a single bearer token.
- **The ceremony gets faked.** Users on newer models report the assistant "pretend[s] to run
  through the algorithm" and admitted editing verification sections "without actually doing the
  work" ([discussion #1312](https://github.com/danielmiessler/LifeOS/discussions/1312)). This is
  the empirical argument for §3: a check that runs cannot be faked; a hook that nags can.
- **Onboarding.** Three hours and a WSL migration for a moderately technical user, against a
  mission statement that promises to serve "not just tech people"
  ([discussion #922](https://github.com/danielmiessler/Personal_AI_Infrastructure/discussions/922)).
- **Everything ships at once.** Business, security research, art, voice, network monitoring,
  Telegram and iMessage bridges, a dashboard daemon — installed for everyone, whether or not
  they have a business or a network.

### 2.3 What this system takes and leaves

| takes | leaves |
|---|---|
| TELOS as typed, sectioned intent | a persona or a name for the assistant — the register it speaks in is the person's own (§7); narratives (the pitch-line form of mission, useful for publishing, not for direction) |
| current state → ideal state, gap as signal | seven fixed dimensions and a coverage percentage |
| claims with named falsifiers | the seventeen-section artifact and the phase machine |
| the interview rule, and the conversation around it (§9) | the 74 hook commands that surround it |
| modules as the unit of capability | shipping every module to everyone |
| a small private record the assistant reads first | a 25 KB constitution |

### 2.4 How others have responded, and where this sits

Surveyed 2026-09-26. Four kinds of response exist, and none of them makes the split §1.1
makes.

- **Forks that keep the whole harness and change the host** — [Universal AI
  Infrastructure](https://github.com/jSydorowicz21/Universal-AI-Infrustructure),
  [pai-opencode](https://github.com/Steffen025/pai-opencode),
  [davdunc/pai-framework](https://github.com/davdunc/pai-framework). They fix portability and
  streamline nothing.
- **The author streamlining himself.** [LifeOS 7.0.0](https://github.com/danielmiessler/LifeOS/releases)
  cut the every-turn doctrine from ~88 KB to ~28 KB and removed modes and tiers under one test —
  "would a smarter model make this rule unnecessary?" — keeping "evidence-based verification, the
  ISA contract, safety gates, exact tool recipes." That is §2.3's "takes" column, reached
  independently. The difference is that there it stays doctrine the model is asked to follow;
  here the same three things are a server operation, a scheduled check and a denial.
- **Memory-first reinterpretations that drop the person.**
  [mnott/PAI](https://github.com/mnott/PAI) builds the `working-memory` profile to the hilt —
  hook-classified observations, pre-compaction summaries, a temporal knowledge graph, layered
  injection — with no goals, values or review.
  [obsidian-second-brain](https://github.com/eugeniughelbur/obsidian-second-brain) is one vault
  shared by seven agents that rewrites itself on ingest, reconciles contradictions on a schedule,
  and enforces one rule with a linter: "every stored fact must be timeless, dated, or a pointer."
  Also no goals, values or interview.
- **Lean vault templates that keep the reflection and drop the enforcement.**
  [seandavi/lifeos-template](https://github.com/seandavi/lifeos-template) has the best
  feedback layer in the field — an audit that flags stale goals and decisions missing their
  revisit date, a decision format with alternatives, a prediction at 30/90/365 days, confidence
  and a revisit date, and a forced quarterly rewrite — all as skills a person runs.
  [kcwoodfield/LifeOS-OSS](https://github.com/kcwoodfield/LifeOS-OSS) says of itself that
  "nothing is enforced."
- And one small thing that is nearly this system's shape with none of its mechanism:
  [pct-mcp-server](https://github.com/mikhashev/pct-mcp-server), a sectioned personal context
  over MCP with per-section access control — §11's audience, months earlier — written by the
  model on request, unverified, four commits.

What is distinctive here, in descending order of confidence: authorship enforced by the server
(§9); claims verified against external evidence, with results that tell a fault from a failure
and open work from both (§8.1); the repository
as the trust boundary (§3.8); the 2 KB cap as a constraint (§10). Where others are ahead:
automatic capture of working memory (mnott), self-maintenance of the store (obsidian-second-brain),
and decision capture with a revisit date (seandavi). This specification takes the ideas from the
last two that fit (§1.1's freshness rule, §7's `decision` kind) and leaves the rest open (§15).

## 3. Principles

These invert LifeOS's choices where the evidence in §2.2 says to.

1. **The person does not have to remember anything.** Freshness, cadence and verification are
   the system's job. Where a step would rely on the person, it is automated or backstopped.
2. **The install touches nothing it does not own.** Registering the plugin is the only change
   to the assistant's global configuration: it adds no permission allowlists, no auto-approval
   modes and no hooks beyond the two in §4.2. §2.2 shows what happens otherwise: configuration
   the install rewrites is configuration that later fails silently.
3. **The server is the security boundary, not the model.** Scope is enforced on read; writes are
   validated against a schema; secrets are refused at the point the record is stored. The model
   is asked to be sensible and is not relied on to be.
4. **Every enforcement is a denial or a check that runs. Never a warning.** A warning is
   advisory, and advisory mechanisms are what the record of §2.2 shows failing.
5. **No shell in claims.** A claim's evidence comes from a declared adapter with declared
   arguments, so the person's goals never carry executable code, and nothing in the record can
   become a command (§11).
6. **Core is what everyone has:** identity and values, goals, health and money. Everything else
   is a module, and no module is installed by default, because shipping everything to everyone is
   one of the things that made the prior art unwieldy (§2.2).
7. **Nothing personal ships.** The system is public and the record is private, so the two
   never share a repository.
8. **A record's trust domain is its repository.** Access to a git repository is per
   repository, and a clone is the whole tree. The kernel's audience filter (§11) is the boundary
   above the store; the repository is the boundary below it, and the stronger one. Records meant
   for different sets of people never share a repository, so a personal record and a memory
   shared with anyone else are always two repositories.
9. **The kernel has no product semantics.** Profiles are the kernel's and closed; kinds are the
   module's and open. Nothing about what a record *means* is decided anywhere else.

## 4. Architecture

The system is one kernel, two profiles and a set of modules, all in this repository. A
deployment is the kernel plus the modules it enables: the memory server this was built on is the
kernel plus `memory`; a personal record alone is the kernel plus the four core modules; and both
together are one instance with all five.

### 4.1 The kernel

The kernel is the server. It owns the record and is its only writer. The first three rows below
are the server this system was built on; the rest are added by this specification, and §14 says
which milestone delivers each. None of it knows what a `goal` or a `trap` is.

| responsibility | does |
|---|---|
| store | one markdown file per record, frontmatter composed by the server, committed and pushed to one private git repository; writes validated against the record's module schema |
| retrieval | hybrid lexical + dense search, scope-filtered on read; `working-memory` records are searched by default, `ratified-record` ones only when asked, and the notebook (§5) when named |
| identity | who is calling, from the network layer or a token |
| **profiles** | the closed set in §1.1; each module's manifest names one and the kernel enforces its bundle — who may write, whether records are rendered or searched, whether `review` is required, the audience default |
| **modules** | loads module manifests; exposes each module's kinds, interview prompts and summary template, and serves the module set to the plugin through a read-only `modules` tool, which lists to a caller only the modules it may read (§11); refuses a manifest that does not validate |
| **context** | renders a size-capped session block from the enabled `ratified-record` modules' templates |
| **claims** | runs the evidence adapters itself, on the deployment's interval, and stores what each measured — pass, fail or no evidence, each with its own timestamp, and a count where the claim counts — through the kernel's claim-result operation; what a result means today (`open`, `behind`, `fail` and the rest) is derived from that and the date whenever it is read (§8.1) |
| **agenda** | computes what is due — failed claims, claims behind, drafts, stale records, onboarding — and renders the top item, with its question, as the first line of the context block (§9) |
| **review** | a distinct operation carrying the question asked, a verdict and the person's answer in their own words; the only path that moves `reviewed` (§9) |
| **reflect** | a read-only tool that computes the gap by value — each value, the goals that serve it with their claim states and days since confirmed, and the goals that serve none — for the interview to phrase (§9) |
| **budgets** | validates each enabled module's byte budget against the cap on load; refuses overflow rather than truncating (§10) |
| **audience** | enforces per-module readability per consumer, so a read-only consumer cannot read a module the person has kept to their own sessions (§11) |
| **view** | a read-only HTML rendering of context, freshness, claim state, revision lines and snooze counts |
| **credential refusal** | a write matching a small set of credential shapes is rejected before it reaches git (§11) |

### 4.2 The plugin

The plugin is the assistant-side client. It has at most two hooks and a small set of skills,
because every hook is configuration the install adds to the assistant (§3.2).

| hook | does |
|---|---|
| `SessionStart` | always: fetches the context block from the kernel and injects it; drains the offline outbox; resolves the scope keys a `working-memory` module asks for (machine, project) |
| `PreToolUse` | only when a `working-memory` module is enabled: denies writes to the assistant's built-in per-machine memory path, so working notes have one home. On a deployment with no working memory the model keeps its built-in scratch, and this hook is not installed |

| skill | does |
|---|---|
| `/interview` | holds the interview: a conversation, in the person's register, that turns what they say into drafts and confirms them through `review` (§9) |
| `/done` | scaffolds a done-statement for a piece of work before it starts (§8.2) |
| `/health` | reports whether the guard is active, the outbox is empty and the server is reachable. It also reports the four failures that would otherwise be silent — a module over its byte budget, an outbox write rejected at drain, the claim scheduler not having run, and an adapter erroring — and runs the `working-memory` freshness lint, which lists records that are neither timeless, dated nor pointers (§1.1) |

Modules may contribute skills; the kernel lists them and the plugin loads them.

### 4.3 Modules

A module is a directory with a manifest (§6). Five ship: `memory` under `working-memory`, and
the four core modules under `ratified-record` (§7). A module is enabled per deployment; a
disabled module's data remains stored — it is markdown — and is simply not written, searched,
interviewed, summarised or verified. Nothing is installed by default beyond what a deployment
enables.

## 5. The record

A record is one fact per file, with YAML frontmatter composed by the server. An index file is
maintained by the same write. A private git repository is the only persistence, and pull, write,
commit and push, in that order, is the only write path. All of that is the server this system is
built on.

This specification adds four fields:

| field | meaning |
|---|---|
| `module` | which module's schema this file obeys, e.g. `telos` |
| `kind` | the module-defined kind, e.g. `goal`, `belief`, `account` |
| `id` | a stable short identifier for kinds that need to be referred to across edits (`G3` stays `G3` when edited or retired) |
| `reviewed` | the date the person last confirmed the content. Moved only by the `review` operation (§9), never by `write`; distinct from `updated`, which any write bumps |

A record in a `ratified-record` module lives at `<module>/<kind>/<slug>.md`. A `working-memory`
module declares its layout (§6): `kind`, the same rule, or `free`, the store's existing tree, in
which case the module also declares the scope keys the plugin resolves. There is no unstructured tier:
every record belongs to a module, and a file without one is refused.

**One repository per kernel instance, in this version.** A kernel serves one root, which is one
private repository. Records for a different set of people are a different repository (§3.8) and
therefore a different kernel instance; the assistant's client may register more than one kernel,
which is how a shared memory would sit beside a personal one without either holding the other's
records (§15). A kernel serving several roots is a kernel change, not a configuration.

**The notebook is the one exception, and it is read-only.** A deployment may point the kernel at
a second repository: the person's own notes, written by the person, by the assistant at their
request through its own tools, and by whatever editor they use. The kernel reads and searches
it, named by the tools' `repo` argument, `notebook` (`projects`, its earlier name, is still
accepted), and never writes to
it. It has no modules, no scopes and no audience of its own, and its files keep whatever
conventions the person uses; nothing in it is a record in §5's sense. It is not a profile, because
profiles govern records the kernel writes (§1.1). Because it is never written, it leaves the
one-writer rule intact; because it has no audience, it is refused to consumers outright (§11).
Its search is lexical unless the deployment sets `BRABEUS_NOTEBOOK_EMBED=1`, which adds the dense
leg. The default is off because the notebook is much larger than the record, and its first
embedding pass takes minutes rather than seconds (§16 AW).

**Existing records are migrated by the kernel, once.** A record with the pre-module `type` field
and no `module` is rewritten by the server — the only writer — to `module: memory` and the
matching `kind`, in one commit, with search verified before and after. Nothing is left untagged.

Scope is one of `global`, `project/<remote-slug>` or `machine/<host>`, enforced on read, as it
was in the server this is built on. Every record carries a scope, `global` by default. The plugin
fills in machine and project automatically for a module that declares `scope_keys`, and in this
version only a working-memory module may declare them. A ratified record is `global` unless
written with a scope, as a crossing preference keeps the one it had in `memory`. Scope says
*where* a record applies. It does not say *who* may read it; that is the module's audience
(§6, §11), and the two are enforced independently.

A write whose content matches the record makes no commit and leaves `updated` alone, because
writes are repeated (the outbox replays them, and the assistant re-saves notes it already holds)
and a repeat must not read as a change: on a ratified record it would show a revision that never
happened. A write that would change a record the person has already confirmed is refused
(§9).

**A write whose fields do not match the module's schema is rejected.** The current server already
makes malformed frontmatter impossible by composing it; this extends the same guarantee to module
data.

## 6. The module contract

Each module has a `module.json` at its root. It is JSON rather than YAML so that the kernel can
parse it with its standard library. A key the kernel does not know refuses the module, because an ignored key is a declaration the author believes is in force
and is not: a misspelling, or a key from a newer kernel, would otherwise change behaviour
silently. A manifest looks like this:

```json
{
  "name": "telos",
  "version": 1,
  "profile": "ratified-record",
  "priority": 10,
  "budget_bytes": 600,
  "audience": "self",
  "intro": "direction: what you are here to do, and what you are working toward",
  "kinds": {
    "goal": {
      "fields": ["id", "title", "ideal", "by"],
      "optional": ["claims", "serves", "notes"],
      "freshness_days": 90,
      "interview": "Still right? Progress since {reviewed}?",
      "first": "What are you working toward, and by when?",
      "lenses": ["What would you be proud to have done by this time next year?",
                 "What keeps coming back to you as something you mean to get to?"],
      "draft": "Is this the goal as you would put it?"
    },
    "belief": {"fields": ["statement"], "freshness_days": 365, "interview": "Do you still hold this?"}
  },
  "summary": "summary.md.tmpl",
  "adapters": ["tracker", "forge", "date", "manual"],
  "skills": []
}
```

The kernel validates the manifest on load and refuses a module whose manifest it does not
understand, and refuses to start if the enabled modules' `budget_bytes` sum to more than the cap
less the agenda line's reservation (§10). A module's summary template receives only that
module's records, already scope-filtered, and may select on `reviewed` so that confirmed
records can be preferred over inferred ones. Templates are logic-light by design: they select
and format, they do not compute. Templates get only the `first`, `date` and `age` functions,
because a template ships with a module and must only format the records it is given: a function
that could read the environment or a file would let a module template print a deployment's
secrets into the block. A new function is a kernel change, reviewed like one.

`audience` defaults to the profile's default (§1.1). A module that wants to be readable by other
consumers says so; a core module may not.

**What a manifest may and may not decide**, stated once:

| the module declares | the profile fixes |
|---|---|
| kinds, their fields, ids and freshness thresholds | who may write, and whether `review` is required |
| interview prompts, lenses and summary template | whether records are rendered or searched |
| adapters, budget, priority, layout keys | the audience default, and for core modules the audience itself |
| which profile it runs under | what the profile means |

Which keys a manifest declares depends on its profile. A `ratified-record` module declares
`budget_bytes` and `summary` and may not declare `scope_keys`, `layout` or `legacy_types`. A `working-memory` module declares none of the
former — it is never in the block — and may declare `scope_keys`, a `layout` of `free` (the path
rule is the store's own tree) or `kind` (`<module>/<kind>/<slug>.md`, the default), and
`legacy_types`, the map the one-time migration (§5) uses. Per kind, `interview` and
`freshness_days` are optional. A stale record whose kind declares no `interview` prompt is asked
"Is this still right?", so no module leaves the first line with nothing to ask. A kind without
`freshness_days` is never asked about because of its age; its records still reach the agenda as
drafts, through a failed claim, or through its `due_field`, because some kinds are better asked
about on a date than by age. `due_field` names a date field that makes a record due once the date
has passed and the record has not been reviewed since; the agenda lists it as `stale`, with the
kind's `interview` question. `first` is the question asked when nothing of the kind is on file
(§9); `timeless` exempts the kind from the freshness lint (§1.1). A `ratified-record` module's
`onboarding` lists the kinds to ask for, in order, when none of that kind is on file; each named
kind must carry `first`, because onboarding asks exactly that question. `id` is the one reserved
record field a kind may also declare, so that a kind whose records are referred to across edits
can require one (§5).

The manifest also declares the interview's scaffolding. For each `ratified-record` kind,
`lenses` lists two to four ways into the kind for a conversation — questions that lower the bar
where `first` asks directly — and `draft` is the question asked of a record of that kind that has
never been confirmed ("Is this right as written?" when a kind declares none). For each module,
`intro` is one line the interview uses when it turns to the module. The kernel validates these
keys and attaches no meaning to them, because how to ask belongs to the module and what is due
belongs to the kernel (§3.9, §9).

A module never carries credentials. If a module's adapter needs a token, the token is the
deployment's configuration and the adapter reads it from the environment.

## 7. Core modules

| module | profile | kinds | what it holds |
|---|---|---|---|
| `memory` | `working-memory` | `note`, `trap`, `preference`, `project`, `thread` by default; a deployment renames or extends them | the assistant's working notes — what it learned, what bit it, how a project is laid out. This is the existing server's content, and it is the only shipped module that declares scope keys (§5) |
| `identity` | `ratified-record` | `register`, `fact`, `belief`, `value`, `preference` | who the person is, what they hold true, how they want to be worked with and spoken to |
| `telos` | `ratified-record` | `mission`, `goal`, `problem`, `challenge`, `strategy`, `current`, `ideal`, `decision` | direction: what they are here to do, what they are working toward, what gets in the way, where they are and where they want to be — and the decisions they made on the way |
| `health` | `ratified-record` | `baseline`, `condition`, `routine`, `metric` | the honest current state of the body, and what is being done about it |
| `finance` | `ratified-record` | `account`, `obligation`, `target`, `metric` | what there is, what is owed, what is aimed at |

The four `ratified-record` modules are core in the sense of §3.6 — everyone has an identity,
goals, a body and money. `memory` is core in a different sense: it is what the kernel was built
for, and a deployment that wants an assistant with working notes enables it. A deployment that
wants only the personal record does not, and needs no guard (§4.2).

`current` and `ideal` in `telos` are free-form per dimension, and the dimensions are the person's
own, declared as they write them. There is no fixed list and no coverage score. The gap is shown
as the two texts side by side, dated.

A goal names the values it serves (`serves`, §6). That is what lets the interview reflect the
gap by value rather than by goal (§9), which is the system's stated purpose (§1). A goal that
serves no named value is allowed, and the interview says so when it reflects.

A `decision` is a claim the person makes about themselves, captured before the outcome is known:
what was decided, the alternatives rejected, a prediction, a confidence, the worst case, and a
`revisit` date. The kind names `revisit` as its `due_field` (§6), so when the date passes the
decision is on the agenda (§9) and the interview asks whether the prediction held — which is the one question a
person almost never asks themselves unprompted. The format is
[seandavi/lifeos-template](https://github.com/seandavi/lifeos-template)'s `/decide`, adopted
because it is the best prospective-decision format in the field and because a decision with a
revisit date is the only kind of goal that comes with its own falsifier.

The four core modules are `audience: self`, and no manifest may change that, because they are
the person's own record — who they are, what they value, what they are aiming at, their body and
their money — and a consumer is untrusted by definition (§11). They are private without exception.
Something meant to be shared, such as goals the person wants another agent to see, belongs in a
separate module declared `any`, such as a `telos-shared`; the core record stays whole.

`identity` and `telos` are two modules, not one, because several things are set per module and
the two use them differently: each has its own budget, so a long list of goals cannot crowd out
who the person is; identity renders and is asked first; and each has its own onboarding and
`intro`, which matters because modules are the topics of getting to know you (§9) and "who you
are" and "where you are going" are two different conversations.

`preference` is the one kind that exists in both profiles, and the profiles make the overlap
exact. A record of how the person wants to be worked with — terse answers, report before acting
— is needed by every session at start, and `memory` is where the model writes one when it learns
it. The interview reviews `memory/preference` records like any `identity` kind; a `review` sets
`reviewed` on the same file, and from then on it renders in the block as `identity/preference`.
Model proposes, person ratifies, one file. No other kind crosses.

In a deployment without `identity`, a `memory/preference` is an ordinary working-memory record: it
is searched, never asked about and never rendered, and `review` refuses it, because its governing
module is `memory`. A `reviewed` stamp it already carries stays in the file and takes effect
again if `identity` is enabled, so the preference renders as the confirmed one it was without
being asked again.

`register` is how the person wants to be spoken to — dry or warm, formal or loose, how much colour
— as a ratified statement that renders in the block, so every session uses it. It is not a persona:
it names no character and gives the assistant no name (§2.3). It is first in `identity`'s
onboarding, so getting to know the person starts by agreeing how to talk.

`thread` is the note the interview leaves when the person raises something no enabled module can
hold (§9): the topic, the date, and in its `belongs_to` field the module the interviewer judges it
belongs in. That is a guess: the module need not exist yet, and may never exist under that name.
So a thread is picked up by meaning, not by name. When a module is enabled, the interview looks
for threads whose topic the module covers, whatever they name, and develops each into records of
that module; once the module holds at least a draft from it, the thread is deleted. Nothing else
ends a thread, so one that no enabled module covers stays on file.

A thread whose `belongs_to` names a core module with `audience: self` holds a pointer only, and the
kernel refuses it a body; the substance waits for its ratified home. The kernel knows the core
modules whether or not they are enabled, so the check needs nothing else. A thread naming any
other module, known or not, keeps what the person said as a summary. The check cannot see what
the name and description carry; keeping those to the topic is the interviewer's part.

Health and finance ship with `manual` as their only adapter in v1. Wearable, bank and calendar
adapters come later. Each is added to the kernel as an interface with its own backends, and health
or finance declares it, because a module is data and must not carry code that calls an outside
service (§3.5, §8.1).

## 8. Intent engineering

Intent is written down at two levels, the goal and the piece of work, and both use the same
mechanism: short, binary claims, each naming what would show it false.

### 8.1 Goals carry claims

A `telos/goal` may carry claims: short, binary statements of what true would look like, each
naming its evidence.

```yaml
claims:
  - text: "At least three articles published this quarter"
    check: { adapter: tracker, done: true, label: article, since: 2026-07-01, min: 3 }
  - text: "The first draft of the guide is written"
    by: 2026-11-15
    effort: 21d
    check: { adapter: manual }
  - text: "Every photo from 2025 has been reviewed"
    check: { adapter: manual, of: 1200, since: 2026-09-01 }
  - text: "The side project has a commit in the last fortnight"
    standing: true
    check: { adapter: forge, repo: side-project, since: -14d, min: 1 }
  - text: "The target date still holds"
    standing: true
    check: { adapter: manual }
```

`standing`, `by` and `effort` are keys of the claim, beside `text`, because they say what the claim
means. `of` and `since`, like `min`, are arguments of its `check`, because they say how it is
measured.

The system runs every non-manual claim on the deployment's interval, daily by default, and
records what it measured against the goal: one of three states, each with its own timestamp, and
for a counting claim the count and the target it was compared with.

| measured | means |
|---|---|
| `pass` | the adapter returned evidence and the claim held |
| `fail` | the adapter returned evidence and the claim did not hold |
| `no-evidence` | the adapter could not answer — a credential expired, the source was unreachable, the query matched nothing it could count |

**End-state and standing claims.** Most claims say what will be true by a date: "the six tasks
are done", "every photo has been reviewed". Before that date, a claim that does not hold yet is
work still open, not a contradiction. Some claims say what should hold all the time: "a commit in
the last fortnight", "the target date still holds". One of those not holding is a contradiction
now. A claim is *standing* when it says `standing: true`, and a `date` claim is always standing,
because its `before` and `after` are already its dates. Every other claim is *end-state*. Standing
is said, or follows from the adapter, never inferred from a claim's arguments: a reader years later
should not have to know that a rolling `since` changes what a claim means. So a claim with a
rolling `since` (`-14d`) must say `standing: true`, and the kernel refuses one that does not,
because a rolling window has no start to pace from.

An end-state claim's deadline is its own `by`, which may not be later than the goal's, or else the
goal's `by`, so a goal can set milestones ahead of its own date. Dates are compared in the
kernel's timezone, its `TZ` setting, UTC when unset, so that a deadline ends when the person's day
ends rather than when UTC's does.

**Size.** How soon an unmet end-state claim needs attention depends on how much work it is, so
each may say how big it is:

- a *counting* claim — `tracker` with `done: true`, or `forge` — counts completed work. With a
  `min` of 2 or more it measures its own progress: the count against `min`, over the window from
  `since` to the deadline, and it must name `since`, because progress needs a start;
- a *manual counting* claim names its total with `of` and the start of its window with `since`,
  both required. Each answer records the count so far as a number beside its note, never inside
  it, so the kernel need not read prose, and the claim passes when the count reaches `of`;
- every other end-state claim is *yes-or-no*, because its work is either done or not: a manual
  claim without `of`, a counting claim whose `min` is 1 or absent, and a tracker claim counting
  open tasks, whose count can fall as well as rise and so is not progress. It may carry `effort`:
  the person's estimate of how long the work will take them to get done, given the rest of their
  life, as a number of calendar days written `<n>d`. Three days of work done over three weekends
  is `effort: 21d`, not `3d`.

A standing claim takes no `by` and no `effort`, because it has no deadline to work toward.
`effort` belongs only to a yes-or-no claim, because a counting claim's pace is measured rather
than estimated, and `of` only to a manual claim, because an adapter's total is its `min`. The
kernel refuses a claim that breaks these rules, or a deadline that is not a date — the claim's
`by`, or the goal's when a claim takes its deadline from it — when the goal is written, naming
the claim by its position, as it refuses any other bad claim.

**What a result means today** is derived whenever it is read, from what was measured, the claim's
keys and the date. So a claim moves from open to behind, or to failed, on the day the arithmetic
says so, not at the next run, and a manual claim, measured only when the person answers, moves
with the calendar like the rest. An end-state claim with nothing measured yet counts as not met,
so a manual claim nobody has answered is raised by its deadline like any other; a standing claim
with nothing measured yet is `unchecked` until it is.

| state | when |
|---|---|
| `pass` | the evidence holds |
| `open` | an end-state claim not met yet, with its deadline ahead and its work on pace |
| `behind` | an end-state claim not met yet whose pace says it may miss its deadline: a paced claim whose count is below half the work its window so far would expect, counted in whole items: the expected work and its half are each rounded down, or a yes-or-no claim with an `effort` and no more days left than that effort, counting the deadline day as one |
| `fail` | a standing claim not met, or an end-state claim still not met after its deadline day |
| `no-evidence` | as measured: a fault in the deployment |
| `unchecked` | a standing claim with nothing measured yet |

A counting claim's expected work assumes the work is spread evenly over its window and is counted
in whole items, rounded down, and so is its half, because a claim cannot be behind on a fraction of
an item: nothing is `behind` until a whole item of that half is due. Half the expected work is a deliberately loose test, because
work arrives unevenly, and a test that fired at the first quiet week would teach the person to
ignore it. A yes-or-no claim without an `effort` gives no early warning at all and fails only on
its deadline, because a warning computed from an estimate nobody made would be a guess presented
as a measurement. Where the arithmetic cannot run, the claim is raised rather than hidden: a
deadline already on file that does not parse reads as `behind`, and its agenda item says the date
could not be read, so the person corrects the date rather than the work. A claim already on file
with a rolling `since` and no `standing` reads as `behind` too, and its agenda item asks whether it
should hold all the time, because a rolling window has no start to pace from. A result recorded
before counts were stored has no count, and its claim reads as a yes-or-no claim without an
`effort` until the next run, or for a manual claim the next answer, measures it.

Only `fail` is a contradiction, and `behind` is the warning before one; `open` is neither.
`no-evidence` is a fault in the deployment, reported by `/health` and shown in the view, and it
never opens an interview as if the person had fallen short. A `pass` older than two intervals is
shown as stale, not as passing, so one missed run does not change every goal's state but a
scheduler that has stopped shows within two days.

A `manual` claim is asked at interview, and its answer is recorded with the kernel's
claim-result operation, like an adapter's result. The answer's note is written to make sense on
its own, quoting the person where their reply needs the question beside it, because the agenda
line and the reflection show the note without the question that drew it. An open manual claim is
not on the agenda, but a check-in on its goal asks it, so a manual count is kept current before
it falls behind. A review of the goal happens only if the person also confirms or corrects the
goal itself, so whether the evidence is in stays separate from
whether the goal is still right. A manual `fail` ranks equally with an adapter `fail`, so modules
without adapters are not second-class.

Results are written by the kernel's claim-result operation; they never change a goal's content
or its `updated` stamp. What the person said about a goal is theirs, and whether the evidence
agrees is the kernel's, so a result must not make a goal look revised (§9's revision line) or fill
its history with results. Results are stored as one JSON file per goal under `claims/`, at the
goal's own path; that file is not a record, and it is never listed or searched (§16 AW). The
kernel runs the schedule itself on `BRABEUS_CLAIM_INTERVAL`, and `0` turns it off (§16 AV).
Results reach the record only through the kernel, so there is still one writer.

The claim carries no code: the adapter is named, the arguments are data, and the kernel
refuses an adapter the module did not declare.

The adapters in this version are `tracker` (a task tracker: counts of done or open items by label and date),
`forge` (a git forge: commit and merge counts by repository and date), `date` (before, after,
between), `manual`. Each is an interface with one implementation per backend, configured per
deployment.

### 8.2 Work carries a done-statement

`/done` scaffolds a short document for a piece of work before it starts — four sections:

1. **Goal** — one to three sentences, the person's words kept verbatim.
2. **Out of scope** — what this is not.
3. **Claims** — binary, each with a falsifier. Where the falsifier is a command, it is written as
   a fenced `check` block the working repository's tooling can execute; where it is a judgement,
   it says so.
4. **Decisions** — dated, including dead ends.

That is the whole artifact. There is no stop-time gate that refuses to end a session with open
claims; the claims that can run, run, and the ones that cannot are visibly unverified. §2.2's
evidence is that gates get performed rather than obeyed.

## 9. The interview

The interview is the mechanism that keeps the record true, and the way it is first written. The
kernel makes two decisions — what is due, and what counts as reviewed — and the interview is a
conversation built on them. This is §3.3 and §3.4 applied to the loop that matters most: a cue
addressed to the model is advisory, and the field the freshness signal runs on must not be movable
by a write the kernel cannot attribute. What the conversation does with those two decisions is not
the kernel's business. An interview that reads the kernel's question aloud and files one answer per
record reads as a form, and a person abandons a form. What the prior art got right (§2.1) was the
interview rule and also the conversation around it: an advisor getting to know the person, who asks
after what they mean as well as what they said.

**The agenda.** The kernel computes it from fields the record already has, so it is stateless:

1. claims in `fail` (§8.1) — standing claims not met and end-state claims past their deadline,
   adapter and manual alike — ordered by the claim's deadline, or for a standing claim the goal's
   `by`, nearest first, and then by how long the claim has been failing, because a goal due soon
   still has time to act on;
2. claims `behind` (§8.1), ordered by the claim's deadline, nearest first, because a warning is
   worth most while there is time to act on it — except a claim whose goal was reviewed, or put
   off with `later`, in the last 7 days, which waits between the stale records (4) and onboarding
   (5) instead. The person has just been asked about that goal; a warning kept on the first line
   after they have answered it would hold the one slot every session for as long as the work is
   slow, and teach them to ignore the slot. Seven days is a week of sessions: long enough for the
   line to move on, short enough that slow work comes back. A deferred claim still heads the line
   when nothing else is due, because then there is nothing it keeps waiting. The kernel records
   when a record was last put off, beside its snooze count, so a `later` defers it as a review
   does. An `open` claim is never on the agenda: work with its time still ahead of it is not
   something to ask about;
3. drafts: records governed by a `ratified-record` module that have never been reviewed, oldest
   first — approvals left over from an earlier conversation, and preferences the model wrote in
   `memory`, which are due as soon as they are written;
4. records past their kind's `freshness_days`, or past the date their kind's `due_field` names,
   ordered by module priority and then by age;
5. onboarding: kinds a module lists in `onboarding` of which nothing is on file, in module
   priority and then `onboarding` order;
6. nothing, if none of these exists.

Within each reason, preferences sort last, native and crossing alike: a preference about how the
assistant works is one the person changes when it bothers them, so re-confirming one matters less
than the rest of the record, and a preference the model inferred stays behind what the person said
themselves. Onboarding keeps `identity`'s own order, because the first conversation is where
preferences are set.

Each item carries its reason — `fail`, `behind`, `draft`, `stale` or `onboarding` — and the
question for it: the kind's `draft`, `interview` or `first` prompt (§6). A failed or behind claim's
item names what was measured — the failure, or for `behind` the count against its expected work, or
the days left against the `effort` — and then asks the goal's `interview` question, or its `draft`
question if the goal has never been reviewed, because a draft has no review to measure progress
from. `no-evidence` and `unchecked` claims are not on the agenda: one is a fault and the other is
not yet measured (§8.1). A record the person has snoozed stays on the agenda with its snooze count,
so a "later" is visible rather than silent.

**The first line of every session** is the top agenda item, rendered by the kernel with its
question — *"G3, 'ship the guide by October': the claim 'three articles this quarter' is behind:
none of 3 found, 2 expected by now, 10 days left. Still right?"* — and, when the record has been
revised since it was last reviewed, its revision line from git: *"target lowered from 3 to 2 on
2026-09-04."* It is one item, never a list: a single question is something the assistant can raise
and the person can answer in a sentence, while a list is a backlog the person has to sort; the rest
of the agenda waits behind it for the interview, and one item fits the line's 256-byte reservation
outside the modules' budgets (§10). That line is what bounds staleness to "the next session"
without anyone remembering anything (§1), so the assistant raises its question once, early in the
session, waiting for a natural break if the person opened with a task. It does not raise it again
in that session once the person has answered, put it off or passed over it: a line nobody raises
bounds nothing, and a line raised over and over is a nag the person learns to ignore. There is no
daemon, no status indicator and no notification, and this specification keeps refusing them,
because each is a cue the person would have to notice.

**The `review` operation** takes a record id, the question that was asked, a verdict — confirmed,
corrected (with the new content), retired, or later — and the person's answer in their own words.
It writes the question, the verdict and the answer into the commit, and it is the only path that
moves `reviewed`. The question is the one the person was asked, with nothing the assistant said
around it, and for an agenda item it is the item's question as the kernel wrote it. A plain `write` never does. So a `reviewed` date in history is, by construction,
a question that was asked and answered, and the answer can be read back. `review` accepts only a
record whose governing module is `ratified-record` (§1.1).

**A confirmed record changes only through `review`.** Once a record whose governing module is
`ratified-record` has been reviewed, a `write` that would change its content is refused, and so
is a `delete`. Its content changes only through a `corrected` review, and it leaves the record
only through a `retired` one, which keeps it in history. Without this, a value the person
confirmed could be reworded by the assistant and still render as confirmed, which is the model's
inference presented as the person's (§1.1). A draft, never reviewed, may still be rewritten or
deleted, so a wrong path or an interrupted conversation can be put right. The rule follows the
governing module, so a crossing preference the person has confirmed is frozen too; a refinement
the assistant learns later becomes a new `memory/preference`, which the interview then asks about.

**The conversation.** `/interview` runs in one of two ways, chosen by the top agenda item's reason:
a *check-in* while the top item is a failed or behind claim, a draft or a stale record, and
*getting to know you* when the top item is onboarding. So leftover drafts and claims that need
attention are raised before new topics, as the agenda's order intends, and one conversation can
move from confirming drafts into getting to know the person.

- *Getting to know you* walks topics: modules in priority order, each introduced by its `intro`,
  and within each module the kinds in `onboarding` order, opened with the kind's `lenses`, or its
  `first` question when it declares none. Before it opens a module it looks for threads whose topic the module
  covers (§7), and starts from those.
- A *check-in* opens with the top agenda item and follows the conversation from there, returning
  to the agenda when a topic runs out.

Either way, an answer is sorted into as many drafts as it contains, across kinds and modules,
and each draft is written at once as an unconfirmed record, in the person's words; a draft may be
rewritten until it is confirmed. What the person volunteers is handled the same way, whether or
not anything asked for it. What no enabled module can hold becomes a `memory/thread` naming the
module it seems to belong in (§7). The interviewer labels what it contributes — an inference, a
suggested date, a strategy of its own — so that nothing it added reads as the person's word, and
challenges where it should: an entry that belongs to another
kind, a goal nobody could measure, a contradiction with something on file, a statement that
implies more than it says. Drafts are offered for approval one at a time or together, and each
approval is a `review` carrying the question that was asked and the person's answer. Nothing is
confirmed without one.

**Manner.** The interview speaks in the person's register (`identity/register`, §7), or in a plain
default — warm, direct, short turns — until one is on file. It asks one question at a time and
never asks one whose answer is on file. It is exempt from any preference that asks for terse
answers in working sessions: probing and follow-up are its purpose. "Enough", "stop" and any other
plain request to finish end it at once, and nothing is lost: every draft is already in the record,
and the next session's agenda opens on it. A bare "later" does not end it. In answer to an agenda
item it puts off that item, recorded as a `later` review, and the conversation goes on, because
"later" is the verdict for a deferral and cannot also mean "stop". When the interviewer cannot
tell which the person means, it asks once.

**Reflect back.** When the person stops, or asks for it, the interview closes with the gap
grouped by value, computed by the kernel's `reflect` tool (§4.1) and phrased by the interviewer,
because the reflection is counts and dates that a model could get wrong unnoticed: for each
`identity/value`, the goals that `serve` it and their claims' state, then the goals that serve no
named value. Only `fail` and `behind` are the gap. An `open` claim is reported as work remaining,
with its count and the days left, and never as a shortfall, because the reflection is the
person's progress read back, and work not yet due is not missing. `no-evidence`, `unchecked` and
a stale `pass` are reported as unknown, never as gap or progress. *"You said family time matters
most; the two goals that serve it have not been confirmed in 94 days, and the three that serve
'craft' are all on track."* The gap is the message, delivered without judgement.

**On an empty record** the agenda holds only onboarding items, so the interview is getting to know
the person from its first question — identity first, where the first kind is how they want to be
spoken to, then telos. The first interview populates the record; there is no template to fill in.

## 10. Session context and the view

The kernel renders one block **hard-capped at 2 KB**: first the agenda line (§9), in a reserved
share, then each enabled `ratified-record` module's summary template in priority order,
scope-filtered. `working-memory` modules are never in the block; they are searched (§1.1). The cap
is a design constraint, not a default: a context block that grows is the constitution problem of
§2.2 arriving by another door. A module that cannot say what matters in its share of 2 KB has
not decided what matters.

A record governed by a `ratified-record` module renders marked as unconfirmed until it has been
reviewed, whatever its kind, so a draft never reads as the person's word.

**The cap has an allocation rule, because a shared cap without one is silently truncated.**
Each module declares `budget_bytes` (§6). On load the kernel checks that the reservation plus
the enabled modules' budgets fit the cap and refuses to start otherwise. At render, a module
whose output exceeds its budget is refused — its share renders as one line saying so — and the
fault is reported by `/health`. Nothing is cut without being shown.

The view is the same render as HTML at `/view/`, plus per-record freshness, per-claim state as it
reads today (§8.1), with `open` and `behind` shown apart from `fail`, and age, each goal's revision
line, snooze counts, and the fraction of claims that are `manual` —
the leading indicator that a module wants an adapter. The view is read-only, because anything
that changes state goes through the kernel's validated write (§11). It binds to loopback and
carries no authentication of its own; the deployment fronts it with whatever identity layer it
already runs.

## 11. Security and privacy

- **One writer.** The kernel is the only process that commits to the record, so two writers can
  never produce conflicting changes.
- **Scope on read.** A record scoped to one machine is not returned on another unless asked for
  explicitly. The server enforces this, not a convention the model is asked to follow (§3.3).
- **Secrets refused at the boundary.** The record's git host must scan every push for secrets
  and reject on a hit. The kernel additionally refuses a write that matches a small set of
  credential shapes before it reaches git. Both are boundaries; neither is a warning.
- **Read-only consumers.** Any consumer other than the person's own assistant sessions — an
  untrusted agent, a dashboard, an exchange with another system — gets the read tools and never
  the write tool. A write influenced by an untrusted reader is injection into the next session.
- **Audience, enforced.** Scope says where a record applies; it does not say who may read it.
  A read-only consumer is identified like any caller and sees only modules whose `audience` is
  `any`, on every path that names a module: records, search, the block and the `modules` tool.
  A forbidden module contributes nothing, not even its name, because the name of a module the
  person added is itself something they may not want a reader to know. The four core modules
  are `self` and cannot be made otherwise by configuration (§7). An unidentified caller is
  refused before any tool runs, so it sees nothing. This rule exists because, in the server this system is
  built on, the fail-closed default for an unidentified caller is `global` scope, and `global` is
  where health and finance records live.
- **The repository is the boundary beneath the kernel** (§3.8). Audience is a read-time filter;
  it hides, it does not remove. Anything that must not be in another person's clone is in a
  repository that person cannot clone, which means a separate kernel instance. The kernel never
  offers to filter its way out of that.
- **Ratification is a kernel fact.** `reviewed` moves only through `review` (§9), which
  carries the question and the answer into the commit. The model cannot mark a record confirmed
  by writing it.
- **Nothing executes from the record.** Claims name adapters; templates format; the view renders.
  No path exists by which record content becomes a command.
- **The notebook is refused to consumers.** It has no modules, so no per-module audience can
  stand in for it; a read-only consumer cannot read or search it at all (§5).
- **The view is not a control plane.** It has no POST routes. Anything that changes state goes
  through the assistant, through the kernel's validated write.

## 12. Sharing the system without sharing the person

The repository is public. It contains the kernel with its two profiles, the plugin, the five
shipped modules, a synthetic test corpus and this document. It contains no
real record and no deployment configuration.

Everything that is one person's lives in three places outside the repository: the private record
repository; the deployment's environment (hosts, tokens, adapter endpoints); and the operator's
own notes, which a deployment may also serve read-only as the notebook (§5). A contributor can
run the whole system against the synthetic corpus without knowing anything about any real
deployment.

The onboarding target: a person with an assistant, a git host and one machine runs the kernel in
a container, registers the plugin, and has a working `identity` and `telos` module in one sitting
— with the first interview populating them, not a template to fill in.

## 13. Acceptance test: the reference deployment

The system is accepted when one real deployment passes these, described in the system's terms:

- **The existing store migrates losslessly.** A store of about 150 records with the pre-module
  `type` field is rewritten by the kernel to `module: memory` in one commit; every record is
  still present, and search answers the same paraphrases before and after. The four core modules
  then run beside `memory` on the same instance.
- **A personal record can run alone.** A second instance with only the four core modules
  enabled starts, renders the block, and installs no `PreToolUse` guard; the assistant's
  built-in scratch is untouched on that machine.
- **Proposal becomes ratification on one file.** A `memory/preference` the model wrote, once
  confirmed in an interview, renders in the block as `identity/preference` and its history shows
  one `review` commit on the same path.
- **Three machines, one record.** Sessions on any of them see the same context block, filtered
  by scope, within 2 KB.
- **Claims run.** A goal with `tracker` and `forge` claims shows true results against the
  deployment's task tracker and git forge without the operator running anything.
- **The interview is a conversation.** On an empty record, one answer that touches several things
  becomes drafts in more than one kind and module, each confirmed through `review` with the
  person's own words in the commit; the person's register is agreed first.
- **A topic with no module is kept, not lost.** Something the person volunteers that no enabled
  module holds becomes a `memory/thread`; when a module that covers it is enabled, the next
  interview opens with it, whatever module the thread named.
- **The interview opens with evidence.** On a record where a goal's claim has failed (§8.1), the
  first line of the next session's context is that contradiction, before `/interview` is
  invoked.
- **Open work is not a failure.** A goal whose end-state claims are unmet, with their deadlines
  ahead and their work on pace, has nothing on the agenda and nothing in the reflection's gap; one
  whose count falls below half its expected work is the first line as `behind`; and the day after
  its deadline it is `fail`.
- **A stale record surfaces on its own.** A record left past its `freshness_days` while
  sessions continue appears as the first line of a session without anyone invoking anything.
- **A broken adapter is not an accusation.** With the tracker's credential revoked, no claim
  reads `fail`, the agenda line does not name the goal, and `/health` names the adapter.
- **`reviewed` is attributable.** The record's history shows `reviewed` moving only on
  `review` commits, each carrying a question and an answer.
- **The block never truncates silently.** With module budgets set to overflow the cap, the
  kernel refuses to start; with one module's output over its budget, that module's share says
  so and `/health` reports it.
- **An untrusted reader cannot write, and cannot read what is `self`.** A second consumer with
  read-only access searches the record; its write attempts are refused by the kernel, and
  `health` and `finance` records are absent from its results.
- **The install is inert.** The assistant's global settings differ from before only by the
  plugin registration. Verified by diff.
- **Nothing personal is in the public repository.** A secrets scan and a grep for the operator's
  identifiers over the public tree both return nothing.

## 14. Milestones

| # | delivers | accepted when | status |
|---|---|---|---|
| M0 | the public repository seeded; §12's contents present; §13's last clause enforced on every push by CI | CI runs §13's last clause on every push, and the seeded repository passes it | delivered 2026-09-27 |
| M1 | the two profiles; module contract with `profile`, `budget_bytes` and `audience`; the `memory` module and the one-time migration; `telos` and `identity`; `context` tool with the agenda line; `review`; `SessionStart` injection; the guard made conditional | the existing store migrates and still answers; the 2 KB block renders from real records on all machines; a stale record surfaces as the first line; `reviewed` moves only on `review` | delivered 2026-09-27 |
| M2 | three-state claims and the `tracker`, `forge`, `date`, `manual` adapters; results written through the kernel; the conversational `/interview` (§9) with lenses, drafts, threads and the register; the `modules` tool; `review` carrying the answer; the notebook's dense leg as a deployment switch (§5); reflection by value, through the `reflect` tool; the v1.7 agenda, review, claim and caller changes (§16 AI–AQ and AT–AW) | the first line names a measured contradiction; a revoked credential produces `no-evidence`, not an accusation; a first interview turns the person's own answers into confirmed values and goals, and leaves a thread for anything no module holds | delivered 2026-09-29 |
| M3 | the view | read-only, fronted by the deployment's identity layer; shows revision lines, snooze counts and the manual fraction | planned |
| M4 | `health` and `finance`, `audience: self` | both populated by interview, `manual` claims asked and recorded; absent from a read-only consumer's results | planned |
| M6 | sharing hygiene | §12 and §13's last item pass — continuously, from M0 onward; a second person installs from the README | ongoing since M0 |

M1 is larger than it was, because the profiles and the migration have to exist before any
record is written under the new rules. It is still one milestone: nothing in it is optional.

`/done` (§8.2) is a skill that depends on nothing in the kernel; it shipped with M2.

v1.9's claim dates (§16 BB–BF) are built, ahead of M3, because the view shows claim states and
should not show work that is merely open as failing.

## 15. Open

- Whether the outbox (writes queued while the kernel is unreachable) needs schema validation at
  drain time or only at write time. A rejection at drain is one of the four faults `/health`
  reports (§4.2).
- **Shared memory across people.** A working memory for a group, or a record two people both
  ratify, is not designed. What it would need is known: a third profile — model-written,
  attributed to a person, audience the group, pull — a per-person identity in the kernel where
  today identity is per machine, and a separate repository (§3.8), most likely served by its own
  kernel instance with the assistant's client registering both. Nothing in this design precludes
  it; the profile set is closed, but not finished.
- **A kernel serving several roots.** The alternative to the client registering several kernels.
  Not designed; it is the multi-tenant road and it is not the default.
- **Automatic capture for `working-memory`.** Today the profile depends on the model choosing to
  write a note. [mnott/PAI](https://github.com/mnott/PAI) shows the alternative: a `PostToolUse`
  classifier that turns tool calls into observations, and a pre-compaction summary. Both are
  plugin-side features the `working-memory` profile would turn on, like the guard (§4.2). Not
  designed, and not planned for any milestone: the design question inside it is how much of what the model does should
  become record without anyone deciding it should, which is the same question §1.1 answers "no"
  to for the personal record and has not yet answered for working memory.
- **Self-maintenance of the store.** Duplicates and contradictions accumulate in a
  `working-memory` store; the existing server warns at write time and does nothing after.
  [obsidian-second-brain](https://github.com/eugeniughelbur/obsidian-second-brain) reconciles
  and heals on a schedule. The freshness lint (§1.1) is the first step; a reconcile pass is not
  designed, and it must write through the kernel like everything else.
- **Portability.** The kernel is an MCP server and works with any client that speaks MCP; the
  plugin — the two hooks, the skills — is Claude Code's. Other harnesses would need their own
  thin client. Nothing here prevents that and nothing here provides it.
- **Which preferences the block shows.** `identity` renders one preference, the most recently
  confirmed, because its 450-byte budget has room for no more. A person with several confirmed
  preferences has no say in which one every session sees, and §9's ordering ranks preferences
  last for review, not for rendering.
- **Habituation.** A person reliably failing one claim is reliably greeted with it. v1.9 defers a
  `behind` claim on a goal reviewed or put off in the last week (§9); whether `fail` needs the
  same is unknown until a deployment runs, and the snooze count is the measurement.
- **`/done`'s check blocks** run under "the working repository's tooling" (§8.2), which the
  product does not ship. For a second person they are documentation until a minimal runner does.
- **The bet under the design**, stated as one: that having the gap reflected back changes what
  the person does. Nothing in §13 measures it. What would: whether goals whose claims failed and
  were reflected are more often confirmed, corrected or retired in the following month than
  goals that were not reflected. Unmeasured until M2 has run for that long.

## 16. Changes

**Working name: Brabeus (2026-09-27).** The descriptive names in this document are unchanged;
this section will record the rename when it happens.

### Changes in v1.1

Accepted 2026-09-26, from a systems-thinking analysis of this design.

| | change | sections |
|---|---|---|
| A | the kernel computes the interview agenda and renders the top item as the first line of every session; `reviewed` moves only through a distinct `review` operation. Staleness then surfaces by the next session without anyone remembering it, and a `reviewed` date always means a question was asked and answered | §4.1, §4.2, §5, §9, §10, §11, §13, §14 |
| B | goals name the values they serve, and the reflection is grouped by value, because reflecting the gap between what the person values and what they do is the system's stated purpose (§1) | §6, §7, §9, §14 |
| C | claim results are three-state, each with its own age, and only `fail` is a contradiction, so a broken credential reads as a fault in the deployment rather than as the person falling behind | §4.1, §8.1, §9, §13, §14 |
| D | modules declare a byte budget, and the kernel refuses overflow rather than truncating, because a shared cap without an allocation rule is cut silently | §4.1, §6, §10, §13, §14 |
| E | modules declare an audience, and `health` and `finance` are readable only by the person's own sessions, because scope says where a record applies and not who may read it | §4.1, §5, §6, §7, §11, §13, §14 |
| F | a revised goal shows its revision line wherever its contradiction is shown, so a goal quietly lowered since it was last confirmed is visible when it is questioned | §9, §10, §14 |

Refused, from the same analysis: raising the cap or lengthening `freshness_days`, and any
daemon or notification cadence. The empty-record interview and the wiring of values, open in
v1, are closed by A and B.

### Changes in v1.2

Accepted 2026-09-26, from the operator's own objection: a general memory any agent reads and
writes over MCP has a different goal from a personal context system, and v1.1 had quietly made
the second absorb the first. The evidence — what the existing store holds, who writes each, how
each is read, the audience dimension E had to bolt on, and the prior art's own on-disk split —
is summarised in §1.1.

| | change | sections |
|---|---|---|
| G | the personal context system is a product on the kernel, beside the general memory, not the kernel's future, because the two have different goals. Superseded by L and M: they are now two profiles on one kernel | title, §1.1, §4, §12 |
| H | there is no unstructured tier; a record without a module is general memory and lives in that product's store. The general memory is now the `memory` module (M), so every record belongs to a module | §5, §13 |
| I | M5 is removed: `infra`- and `projects`-style prefixes stay directories inside the `memory` module, whose `free` layout keeps existing paths, because they are working notes and giving them modules would give them the personal record's structure | §14 |
| J | `identity/preference` is named as the one shared kind, readable through both products, because every session needs the person's preferences at its start. Restated by O in terms of profiles | §1.1, §7, §13 |
| K | module-declared layouts are closed, because the case for them was the `projects` prefix; "one process or two" is opened for M1. Layouts returned, per profile, in V | §15 |

### Changes in v1.3

Accepted 2026-09-26, from the operator's second objection: v1.2's "general memory product" had
frozen one deployment's shape — its type taxonomy, its git-remote keying, its scopes — into a
spec that gives the personal side a whole contract and the memory side none. The proposal was to
make the general memory a module and let modules declare the semantic axes; the review kept the
first half and replaced free declaration with kernel-defined profiles, because the axes are not
independent and the guarantees of v1.1 must not be editable from a manifest.

| | change | sections |
|---|---|---|
| L | there are two kernel-defined profiles, `working-memory` and `ratified-record`, each a fixed bundle of author, read model, freshness, audience default and `review` semantics; manifests pick one and cannot edit it, because the axes are not independent and v1.1's guarantees must not be editable from a manifest | §1.1, §3.9, §4.1, §6 |
| M | the general memory is the `memory` module under `working-memory`, with default kinds a deployment may rename, so the memory side has a contract as the personal side does; the kernel migrates the existing store to it once | §4, §5, §7, §13, §14 |
| N | the `PreToolUse` guard is installed only when a `working-memory` module is enabled, so a personal record can run alone and leave the assistant's built-in memory untouched | §4.2, §7, §13 |
| O | the `preference` overlap is defined by the profiles: the model writes under `memory`, the person confirms under `identity`, and it is one file, so ratifying a preference never copies it | §1.1, §7, §13 |
| P | a record's trust domain is its repository, because access to a git repository is granted per repository and a clone is the whole tree; there is one repository per kernel instance in this version; shared memory across people is named as a third profile plus a separate repository, and not designed | §3.8, §5, §11, §15 |
| Q | "one process or two" is closed as one process with one root; "several roots" is left as the road not taken by default | §15 |

### Changes in v1.4

Accepted 2026-09-26, from a survey of other responses to LifeOS (§2.4). Two small things taken
directly; two larger things opened rather than decided, because each carries a design question.

| | change | sections |
|---|---|---|
| R | `working-memory` freshness is no longer "none": a record is timeless, dated, or a pointer, and `/health` lints it, so working notes do not go stale unnoticed | §1.1, §4.2 |
| S | `telos` gains a `decision` kind — alternatives, prediction, confidence, worst case, revisit date — whose revisit date is a `date` claim, because a decision with a revisit date comes with its own falsifier. Replaced by AI: the revisit date is a `due_field` | §7 |
| T | automatic capture for working memory, self-maintenance of the store, and portability beyond Claude Code are opened in §15, because each carries a design question not yet answered | §15 |

### Changes in v1.5

Decided 2026-09-27, while planning M1, because the kernel's loader forced each one.

| | change | sections |
|---|---|---|
| U | manifests are `module.json`, so the kernel parses them with its standard library; an unknown key refuses the module rather than being ignored | §6 |
| V | the path rule is per profile; a `working-memory` module declares `layout` and, for `free`, its scope keys, because the existing store's tree does not follow `<module>/<kind>/<slug>.md` | §5, §6 |
| W | `first` and `timeless` per kind and `onboarding` per ratified-record module, so the interview knows what to ask of an empty record and the lint knows what to skip; `interview` and `freshness_days` become optional | §6, §9 |
| X | `legacy_types` on the `working-memory` module that adopts pre-module records, as the map the one-time migration reads | §5, §6 |
| Y | a `working-memory` module has no budget and no summary, because it is never in the block; the agenda line reserves 256 bytes of the 2 KB, so it always has room | §6, §10 |

### Changes in v1.6

Decided 2026-09-27, from the operator's objection after abandoning the M1 interview: it read as a
form. The prior art's interview felt like an advisor getting to know the person, and §2.1 had
credited its rule without its conversation. AF records a second decision of the same day, from
the domain-driven design analysis in `docs/ddd/`: the kernel's second, read-only repository was
running with no section of the specification describing it.

| | change | sections |
|---|---|---|
| Z | the interview is a conversation in two ways chosen from the agenda — getting to know the person, and a check-in; answers become drafts across kinds and modules; the interviewer labels what it infers and challenges what it doubts; the interview is exempt from terse working preferences, because probing is its purpose | §4.2, §9 |
| AA | drafts are written at once as unconfirmed records, so stopping loses nothing; the agenda gains a `draft` reason and renames `empty` to `onboarding`, which says what it means; every ratified kind renders marked unconfirmed until reviewed, so a draft never reads as the person's word | §4.1, §9, §10 |
| AB | `review` carries the person's answer, in their own words, into the commit, so what they said can be read back | §4.1, §9 |
| AC | manifests gain `lenses` and `draft` per kind and `intro` per module, because how to ask belongs to the module; the kernel serves the module set through a read-only `modules` tool, which withholds from a caller the modules it may not read, as every other path does | §4.1, §6, §11 |
| AD | `identity` gains `register`, how the person wants to be spoken to, so every session speaks as the person asked; §2.3's refusal narrows from the persona, traits and voice to a persona or a name | §2.3, §7 |
| AE | `memory` gains `thread`, so what the person raised that no enabled module holds is kept rather than lost; it is found by meaning when a module that covers it is enabled, developed into that module's records, then deleted. One naming a core `self` module is refused a body, so that detail never sits in the searchable memory module | §7, §9, §13 |
| AF | the notebook — the person's own notes repository, which the kernel lists, reads and searches and never writes — is specified as the one exception to one repository per kernel instance; consumers are refused it, because it has no audience of its own; whether it gets dense search is decided in M2 | §4.1, §5, §11, §12, §14 |

### Changes in v1.7

Decided 2026-09-28, while settling the design gaps found when the documents were reworded so that
each rule carries its reason: places where the specification contradicted itself or the code,
behaviour it left undefined, and rules it stated without a reason.

| | change | sections |
|---|---|---|
| AG | the kernel reads and searches the notebook but does not list it, because `list` shows records with modules and kinds, which the notebook does not have; AF's "lists" overstated it | §5, §11 |
| AH | new adapters are added to the kernel and declared by modules, because a module is data and must not carry code that calls an outside service | §7 |
| AI | a kind may name a `due_field`, a date that makes its records due once it passes unreviewed; `decision` uses it for `revisit`, replacing S's `date` claim, because a date on the record needs no scheduler or written-back result | §6, §7, §9 |
| AJ | §1.1 credits the prior art with keeping working memory and the person's record in separate trees, which is what it does; working memory's audience default is `self`, as in the code; `review` accepts only records governed by a `ratified-record` module, because a working-memory record has no confirmed state; LifeOS's hooks are counted as 74 commands from 60 scripts | §1.1, §2.2, §2.3, §9 |
| AK | a stale record whose kind has no `interview` prompt is asked "Is this still right?", and a kind without `freshness_days` is never stale by age, so no module leaves the first line empty and date-driven kinds are not asked by age | §6 |
| AL | the interview's way follows the top agenda item's reason, so leftover drafts and failed claims come before new topics; a kind with no `lenses` is opened with its `first` question | §9 |
| AM | failed claims are ordered by the goal's `by` date, then by how long they have failed, because goals have no priority field and a goal due soon still has time to act on; within each reason preferences sort last, because the person changes them when they bother them; a preference the model wrote is a draft at once, replacing the wait until its freshness ran out | §9 |
| AN | claims run on one interval per deployment, daily by default, and a `pass` older than two intervals is stale; results are written by the kernel's claim-result operation and never change a goal's content or `updated`, so the revision line stays exact | §8.1 |
| AO | the kernel's `reflect` tool computes the reflection by value, because it is counts and dates a model could get wrong unnoticed | §4.1, §9, §14 |
| AP | a crossing preference in a deployment without `identity` is an ordinary working-memory record whose `reviewed` stamp waits for `identity`; every record carries a scope, and a ratified one is `global` unless written with one | §1.1, §5, §7 |
| AQ | an unidentified caller is refused before any tool runs, matching "sees nothing"; the four core modules are private without exception, with a separate module declared `any` as the way to share | §7, §11 |
| AR | reasons are recorded for rules that had none: a forbidden module's name is withheld, an unknown manifest key refuses the module, the agenda line is one item, identical content makes no commit, and templates get three functions | §5, §6, §9, §11 |
| AS | `identity` and `telos` stay two modules, closing §15's question, because budgets, priority and onboarding are per module; §14 gains a status column and loses the removed M5 row, which §16's I now explains; M6 is ongoing since M0, as its acceptance says; the M2 row names v1.7's M2 changes | §7, §14, §15, §16 |
| AT | a `manual` claim's answer is recorded with the claim-result operation, not through `review`, so whether the evidence is in stays separate from whether the goal is still right | §8.1 |
| AU | once reviewed, a record governed by a `ratified-record` module is refused `write` and `delete`: its content changes only through a `corrected` review and it leaves only through `retired`, because otherwise the assistant could reword what the person confirmed and it would still render as confirmed; a draft may still be rewritten or deleted, and a confirmed crossing preference is frozen like the rest | §5, §9 |
| AV | the kernel runs the claim schedule itself on `BRABEUS_CLAIM_INTERVAL`, and `0` turns it off, closing §15's question; a sibling scheduler would need a kernel change, because the claim-result operation accepts only manual answers and freshness is judged against the kernel's own last run | §8.1, §15 |
| AW | the notebook is searched lexically unless `BRABEUS_NOTEBOOK_EMBED=1` adds the dense leg, because it is much larger than the record and its first embedding pass takes minutes; claim results are one JSON file per goal under `claims/`, not a record, so a result never touches the goal it measures and is never listed or searched. These close the two questions AF and §8.1 left to M2 | §5, §8.1 |

### Changes in v1.8

Decided 2026-10-01, from what the M2 acceptance run showed on the live system.

| | change | sections |
|---|---|---|
| AX | a bare "later" in answer to an agenda item puts off that item and the interview goes on; only "enough", "stop" or another plain request to finish ends it, because "later" is also the verdict for a deferral, and one word cannot mean both | §9 |
| AY | the assistant raises the first line's question once a session, early, and not again once it is answered, put off or passed over, because the line bounds staleness only if someone raises it, and raising it repeatedly would teach the person to ignore it | §9 |
| AZ | a failed claim on a goal that has never been reviewed is followed by the goal's `draft` question, not its `interview` question, because the interview question asks about progress since a review that never happened | §9 |
| BA | a review's question is the question the person was asked, with nothing the assistant said around it; a manual claim's note is written to make sense without its question, because the agenda line and the reflection show it on its own | §8.1, §9 |

### Changes in v1.9

Decided 2026-10-01, because a claim not met yet was read as failed before its deadline, by the
agenda and by the reflection.

| | change | sections |
|---|---|---|
| BB | a claim is end-state, true by a deadline, or standing, true all the time; it is standing only when it says `standing: true`, or always when its adapter is `date`, and a claim with a rolling `since` must say so, because before a deadline only a standing claim that does not hold is a contradiction, and a reader should not have to infer which a claim is from its arguments; `standing`, `by` and `effort` are keys of the claim, and `of` and `since` are arguments of its `check` | §8.1 |
| BC | an end-state claim's deadline is its own `by`, no later than the goal's, or the goal's `by`, so a goal can set milestones; dates are compared in the kernel's `TZ`, UTC when unset | §8.1 |
| BD | an end-state claim may say how big it is: a counting claim of completed work with a `min` of 2 or more names `since` and measures progress against `min`, a manual counting claim names `of` and `since` and its answers carry a count, and a yes-or-no claim may carry `effort` in calendar days; there is no default effort, because a warning from an estimate nobody made is a guess presented as a measurement | §8.1 |
| BE | the stored result is what was measured, and the state it means today (`pass`, `open`, `behind`, `fail`) is derived whenever it is read, so a claim moves with the calendar and not only at a run; `behind` is a count below half its expected work, in whole items rounded down, or no more days left than the `effort`, loose on purpose because work arrives unevenly; an end-state claim never answered counts as not met, a standing one is `unchecked`, and where the arithmetic cannot run the claim is raised rather than hidden | §8.1 |
| BF | the agenda gains `behind` after `fail`, nearest deadline first, and orders `fail` by the claim's deadline; a `behind` claim whose goal was reviewed or put off in the last 7 days waits between the stale records and onboarding, so a slow goal cannot hold the first line every session, and the kernel records when a record was last put off to make that possible; an `open` claim is never on the agenda; the reflection counts only `fail` and `behind` as the gap, reports `open` claims as work remaining and unmeasured ones as unknown | §9, §10, §13, §15 |

## Sources

- LifeOS repository and README — https://github.com/danielmiessler/LifeOS
- TELOS — https://danielmiessler.com/telos
- Building Your Own Personal AI Infrastructure — https://danielmiessler.com/blog/personal-ai-infrastructure
- From Prompt Engineering to Intent Engineering — https://danielmiessler.com/blog/intent-engineering
- The Entire Game for AI Is Articulation of Ideal State — https://danielmiessler.com/blog/ai-ideal-state-articulation
- LifeOS Security — Minimal v2 — https://docs.ourlifeos.ai/Security__README/
- Issues #1860, #2079, #2086, #2087, #2156, #2162; discussions #1312 and (PAI) #922 — linked inline above
- seandavi/lifeos-template — https://github.com/seandavi/lifeos-template
- mnott/PAI (Knowledge OS) — https://github.com/mnott/PAI
- obsidian-second-brain — https://github.com/eugeniughelbur/obsidian-second-brain
- kcwoodfield/LifeOS-OSS — https://github.com/kcwoodfield/LifeOS-OSS
- pct-mcp-server — https://github.com/mikhashev/pct-mcp-server
- Universal AI Infrastructure — https://github.com/jSydorowicz21/Universal-AI-Infrustructure
- pai-opencode — https://github.com/Steffen025/pai-opencode
- davdunc/pai-framework — https://github.com/davdunc/pai-framework
- LifeOS releases (7.0.0, Bitter Pill Engineering) — https://github.com/danielmiessler/LifeOS/releases
- Hilary Gridley, *How to turn Claude Code into your personal life operating system* — https://www.lennysnewsletter.com/p/how-to-turn-claude-code-into-your
- LifeOS v7.40.4 as installed in a container on 2026-09-24: `LIFEOS_SYSTEM_PROMPT.md`, `ALGORITHM/v8.20.2.md`, `skills/ISA/SKILL.md`, `skills/Interview/SKILL.md`, `USER/TELOS/*`, `hooks/*`, `settings.json`

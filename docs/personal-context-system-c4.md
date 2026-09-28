# The personal context system — C4 diagrams

Companion to [`personal-context-system-v1.md`](personal-context-system-v1.md) at v1.6. These
diagrams say the same thing as the spec at four altitudes; where they disagree, the spec wins.
Names are the spec's descriptive ones. M1 is built; the conversational interview of the fourth
diagram arrives with M2.

The diagrams follow the [C4 model](https://c4model.com/): a context diagram for who uses the
system and what it talks to; a container diagram for the separately deployable pieces; a
component diagram for the inside of the kernel; and a dynamic diagram for the one flow that
matters most, the interview. They are Mermaid flowcharts styled by C4 level rather than
Mermaid's own C4 syntax, so they render anywhere Mermaid does.

Legend: a rounded box is a person; a rectangle is a system, container or component at the
diagram's level; a dashed border is something outside the system; a cylinder is a store.

## Level 1 — system context

Who uses it, and what it depends on. The system is one box here; everything else is outside
it and configured per deployment.

```mermaid
flowchart TB
    person(["Person<br/><i>whose record this is</i>"])
    assistant["AI assistant<br/><i>Claude Code sessions on the person's machines</i>"]
    system["<b>Personal context system</b><br/><i>a private record the assistant reads at session start,<br/>verifies against evidence, and reflects back</i>"]
    githost["Git host<br/><i>holds the private record repository;<br/>scans every push for secrets</i>"]
    tracker["Task tracker<br/><i>evidence: done and open items by label and date</i>"]
    forge["Git forge<br/><i>evidence: commits by repository and date</i>"]
    idp["Identity layer<br/><i>fronts the read-only view</i>"]
    consumer["Read-only consumer<br/><i>another agent, a dashboard,<br/>an exchange with another system</i>"]

    person -- "talks to, in sessions;<br/>answers the interview" --> assistant
    person -- "reads the view<br/>(through the identity layer)" --> idp
    idp --> system
    assistant -- "MCP: search, read, write, review, context" --> system
    consumer -. "MCP: search, read, list only;<br/>never write; audience-filtered" .-> system
    system -- "pull, commit, push" --> githost
    system -- "runs claims against" --> tracker
    system -- "runs claims against" --> forge

    classDef person fill:#08427b,stroke:#052e56,color:#fff
    classDef system fill:#1168bd,stroke:#0b4884,color:#fff
    classDef ext fill:#999,stroke:#6b6b6b,color:#fff,stroke-dasharray:5 5
    class person person
    class system system
    class assistant,githost,tracker,forge,idp,consumer ext
```

Two things this level fixes. The person never touches the system directly except through the
assistant or the view. And the read-only consumer's arrow is dashed and one-way: it is
identified like any caller, sees only modules whose audience is `any`, and has no write path
(spec §11).

## Level 2 — containers

The separately deployable pieces. One kernel instance serves one repository; a deployment
enables some set of modules (spec §4, §5).

```mermaid
flowchart TB
    person(["Person"])

    subgraph assistant_host ["The assistant, on each of the person's machines"]
        cc["Claude Code"]
        plugin["<b>Plugin</b><br/><i>SessionStart hook: injects the context block, drains the outbox,<br/>resolves scope keys · PreToolUse guard: only when a working-memory module is enabled ·<br/>skills: /interview, /done, /health, plus any a module contributes</i>"]
        outbox[("Outbox<br/><i>writes queued while the kernel is unreachable</i>")]
        cc --- plugin
        plugin --- outbox
    end

    subgraph kernel_host ["The kernel host — one container, or two"]
        kernel["<b>Kernel</b><br/><i>Go, one static binary · MCP over HTTP · the only writer</i><br/>store · retrieval · identity · profiles · modules · context · claims · agenda · review · budgets · audience · view"]
        embed["Embedding sidecar<br/><i>local model, loopback only,<br/>never published</i>"]
        modules[("Module manifests<br/><i>memory · identity · telos · health · finance ·<br/>any module a deployment adds</i>")]
        clone[("Working clone<br/><i>of the record repository</i>")]
        kernel -- "embeds queries and records" --> embed
        kernel -- "loads, validates" --> modules
        kernel -- "reads, writes, commits" --> clone
    end

    repo[("<b>Record repository</b><br/><i>private git; one per kernel instance;<br/>the trust boundary beneath the kernel</i>")]
    idp["Identity layer"]
    tracker["Task tracker"]
    forge["Git forge"]
    consumer["Read-only consumer"]

    person --> cc
    plugin -- "MCP" --> kernel
    person -- "browser" --> idp -- "/view/, read-only" --> kernel
    consumer -. "MCP, read tools only" .-> kernel
    clone -- "pull / push, secrets scanned at the host" --> repo
    kernel -- "tracker adapter" --> tracker
    kernel -- "forge adapter" --> forge

    classDef person fill:#08427b,stroke:#052e56,color:#fff
    classDef container fill:#438dd5,stroke:#2e6295,color:#fff
    classDef store fill:#438dd5,stroke:#2e6295,color:#fff
    classDef ext fill:#999,stroke:#6b6b6b,color:#fff,stroke-dasharray:5 5
    class person person
    class cc,plugin,kernel,embed container
    class outbox,modules,clone,repo store
    class idp,tracker,forge,consumer ext
```

What the picture is careful about:

- **The plugin is thin.** Two hooks, one of them conditional, and skills. Nothing in the
  assistant's global configuration changes except the plugin's registration (spec §3.2).
- **The kernel is the only writer** to the working clone, and the clone is the only path to the
  repository. A sibling process that runs claims writes its results through the kernel, not to
  the clone (spec §8.1).
- **The embedding sidecar publishes nothing.** It exists because the kernel is a static binary
  and the model call should cross a visible boundary.
- **The identity layer is the deployment's**, not the system's. The view has no authentication of
  its own and binds to loopback (spec §10).
- **Two kernel instances, two repositories** is how a second trust domain — a memory shared with
  other people — would sit beside this one, with the plugin registering both. It is not drawn
  because it is not designed (spec §15).

## Level 3 — components of the kernel

What is inside the kernel box, and which profile each component serves. Everything to the left
of the profiles line is shared by both; everything to the right belongs to `ratified-record`
modules, with the two exceptions noted.

```mermaid
flowchart LR
    subgraph shared ["Shared by both profiles"]
        identity["<b>Identity</b><br/><i>who is calling: network layer or token;<br/>unidentified callers see nothing</i>"]
        audience["<b>Audience filter</b><br/><i>per module, per consumer;<br/>hides, never removes</i>"]
        profiles["<b>Profiles</b><br/><i>working-memory · ratified-record;<br/>closed set; enforces each bundle</i>"]
        modloader["<b>Module loader</b><br/><i>validates manifests;<br/>refuses what it does not understand;<br/>checks budgets sum under the cap</i>"]
        schema["<b>Schema validation</b><br/><i>a write that does not match<br/>its module's kind is rejected</i>"]
        credref["<b>Credential refusal</b><br/><i>credential shapes rejected<br/>before git</i>"]
        store["<b>Store</b><br/><i>one file per record; frontmatter composed here;<br/>index maintained by the same write;<br/>pull → write → commit → push</i>"]
        retrieval["<b>Retrieval</b><br/><i>lexical + dense, fused;<br/>working-memory searched by default,<br/>ratified-record only when asked</i>"]
        migrate["<b>Migration</b><br/><i>one-time: pre-module records<br/>retagged to the memory module</i>"]
        freshlint["<b>Freshness lint</b><br/><i>working-memory: timeless,<br/>dated, or a pointer</i>"]
    end

    subgraph ratified ["ratified-record only"]
        review["<b>Review</b><br/><i>question + answer into the commit;<br/>the only path that moves reviewed</i>"]
        claims["<b>Claims runner</b><br/><i>declared adapters, data-only arguments;<br/>pass · fail · no-evidence, each timestamped</i>"]
        adapters["<b>Adapters</b><br/><i>tracker · forge · date · manual;<br/>one implementation per backend</i>"]
        agenda["<b>Agenda</b><br/><i>fails, then drafts, then stale by priority and age,<br/>then onboarding; snoozes counted; no-evidence excluded</i>"]
        context["<b>Context renderer</b><br/><i>agenda line in reserved space,<br/>then module templates in priority order;<br/>2 KB hard cap; per-module budgets;<br/>overflow refused, never truncated</i>"]
        view["<b>View</b><br/><i>the same render as HTML, plus freshness,<br/>claim state, revision lines, snooze counts,<br/>manual fraction; read-only; loopback</i>"]
    end

    mcp[/"MCP endpoint<br/>search · read · list · write · review · context · modules"/]
    health[/"/healthz + the four silent failures"/]

    mcp --> identity --> audience
    audience --> retrieval
    audience --> store
    store --> schema --> credref
    modloader --> profiles
    profiles -. "governs" .-> store
    profiles -. "governs" .-> retrieval
    profiles -. "governs" .-> review
    claims --> adapters
    claims -- "results, through the store" --> store
    agenda --> claims
    agenda --> store
    context --> agenda
    view --> context
    review --> store
    migrate --> store
    freshlint --> store
    freshlint --> health
    modloader --> health
    mcp -. "modules: the manifests, read-only" .-> modloader

    classDef comp fill:#85bbf0,stroke:#5d82a8,color:#000
    classDef iface fill:#fff,stroke:#5d82a8,color:#000
    class identity,audience,profiles,modloader,schema,credref,store,retrieval,migrate,freshlint,review,claims,adapters,agenda,context,view comp
    class mcp,health iface
```

Reading order for someone new: a request enters at the MCP endpoint, is identified, is
audience-filtered, and only then reaches retrieval or the store. A write passes schema validation
and credential refusal before it becomes a commit. The profile of the record's module decides
whether that write may be a plain `write` or must be a `review`. The claims runner never reads
the request path at all; it runs on a schedule and its results re-enter through the store like
any other write.

## Level 4 — the interview, as a dynamic diagram

The one flow that makes the system more than a notebook. The kernel makes two decisions — what is
due, and what counts as reviewed — and the assistant holds the conversation around them (spec §9).

```mermaid
sequenceDiagram
    autonumber
    participant P as Person
    participant A as Assistant (plugin)
    participant K as Kernel
    participant S as Claims scheduler
    participant T as Tracker / forge

    Note over S,T: on a schedule, independent of any session
    S->>T: run each non-manual claim
    T-->>S: evidence, or nothing
    S->>K: write result: pass / fail / no-evidence, timestamped

    Note over A,K: every session start
    A->>K: context
    K->>K: compute agenda: fails, drafts, stale by priority and age, onboarding
    K-->>A: 2 KB block, first line is the top agenda item with its question and, if revised since last reviewed, its revision line
    A-->>P: the line is in context, and the assistant may voice it

    Note over P,K: /interview, or the person picks up the first line
    A->>K: context, modules
    K-->>A: the agenda, and every enabled module's kinds, fields, lenses and intro
    A->>A: mode: getting to know the person while any onboarding item remains, else a check-in
    loop until the person says enough, stop, or later
        A->>P: a topic's lens, or the top agenda item, in the person's register
        P-->>A: an answer, as long and as wandering as they like
        A->>K: write a draft for each thing the answer holds, in any module, and a thread for what no module holds
        A->>P: the drafts, the interviewer's own inferences labelled, challenges where warranted
        P-->>A: confirmed / corrected / retired / later, in their own words
        A->>K: review(record, question, verdict, answer), once per draft
        K->>K: write question, verdict and answer into the commit, then move reviewed or count a snooze
    end

    A->>K: reflect
    K-->>A: per value, the goals that serve it and their claims' state, then goals serving no named value
    A-->>P: the gap, grouped by what the person said matters
```

Four things the sequence makes visible that prose can hide. The scheduler and the session never
touch each other; they meet only in the store. The kernel decides what is due, and the assistant
decides how to ask about it. Drafts reach the store before anything is confirmed, so a stop in the
middle loses nothing and the next session's agenda opens on them. And `reviewed` moves at exactly
one step, on a call that carries the question, the verdict and the person's words, so the record's
history can be read as a list of things the person was asked and said.

## What is deliberately not drawn

- **A deployment diagram.** C4's fourth standard diagram places containers on infrastructure.
  That is one deployment's business, and the spec is careful to have none in it; the operator's
  companion note is where a deployment draws its own.
- **The module internals.** A module is a manifest, templates, optional skills and optional
  adapters; there is no runtime inside it to diagram. Its contract is spec §6.
- **The second kernel instance** for a shared memory across people (spec §15). When it is
  designed, it is a second copy of the container diagram with a different repository and the
  plugin's MCP registration pointing at both.

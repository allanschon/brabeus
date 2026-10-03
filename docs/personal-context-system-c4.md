# The personal context system — C4 diagrams

These diagrams accompany [`personal-context-system-v1.md`](personal-context-system-v1.md) at
v1.10. They describe the same system as the specification at four levels of detail, and where they
disagree, the specification is correct. Components carry the specification's descriptive names.

The diagrams show the whole design, not only what is built. An element tagged with a milestone,
such as "(M2)", arrives in that milestone, and its arrows are dashed; §14 of the specification says
which milestones are delivered. A tag stays true after delivery, so the diagrams need no update
when a milestone lands. The conversational interview of the fourth diagram is also M2.

The diagrams follow the [C4 model](https://c4model.com/): a context diagram for who uses the
system and what it talks to; a container diagram for the separately deployable pieces; a
component diagram for the inside of the kernel; and a dynamic diagram for the one flow that
matters most, the interview. They are Mermaid flowcharts styled by C4 level rather than
Mermaid's own C4 syntax, so they render anywhere Mermaid does.

In every diagram, a rounded box is a person, a rectangle is a system, container or component at
that diagram's level, a dashed border marks something outside the system, and a cylinder holds
data.

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
    idp["Identity layer<br/><i>fronts the read-only view (M3)</i>"]
    consumer["Read-only consumer<br/><i>another agent, a dashboard,<br/>an exchange with another system</i>"]

    person -- "talks to, in sessions;<br/>answers the interview" --> assistant
    person -. "reads the view (M3)<br/>(through the identity layer)" .-> idp
    idp -.-> system
    assistant -- "MCP: search, read, write, review, context;<br/>the block and the instructions at session start" --> system
    consumer -. "MCP: search, read, list only;<br/>never write; audience-filtered" .-> system
    system -- "pull, commit, push" --> githost
    system -. "runs claims against (M2)" .-> tracker
    system -. "runs claims against (M2)" .-> forge

    classDef person fill:#08427b,stroke:#052e56,color:#fff
    classDef system fill:#1168bd,stroke:#0b4884,color:#fff
    classDef ext fill:#999,stroke:#6b6b6b,color:#fff,stroke-dasharray:5 5
    class person person
    class system system
    class assistant,githost,tracker,forge,idp,consumer ext
```

This level fixes two things. The person reaches the system only through the assistant or the
view, never directly. And the read-only consumer's arrow is dashed and one-way, because a consumer
is identified like any caller, sees only modules whose audience is `any`, and has no write path; a
write influenced by an untrusted reader would be injection into the person's next session
(spec §11).

## Level 2 — containers

These are the separately deployable pieces. One kernel instance serves one record repository,
and may also read the person's own notes as the notebook. A deployment chooses which modules to
enable (spec §4, §5).

```mermaid
flowchart TB
    person(["Person"])

    subgraph assistant_host ["The assistant, on each of the person's machines"]
        cc["Claude Code"]
        plugin["<b>Plugin</b><br/><i>SessionStart hook: injects the context block and the instructions, saves the session's copy, drains the outbox,<br/>resolves scope keys · SubagentStart hook: injects the saved copy into each subagent ·<br/>PreToolUse guard: refuses writes to the saved copies; scratch writes only when a working-memory module is enabled ·<br/>skills: /interview, /done (M2), /health, plus any a module contributes</i>"]
        outbox[("Outbox<br/><i>writes queued while the kernel is unreachable</i>")]
        cc --- plugin
        plugin --- outbox
    end

    subgraph kernel_host ["The kernel host — one container, or two"]
        kernel["<b>Kernel</b><br/><i>Go, one static binary · MCP over HTTP · the only writer</i><br/>store · retrieval · identity · profiles · modules · context · instructions · claims (M2) · agenda · review · budgets · audience · view (M3)"]
        embed["Embedding sidecar<br/><i>local model, loopback only,<br/>never published</i>"]
        modules[("Module manifests<br/><i>memory · identity · telos · health · finance ·<br/>any module a deployment adds</i>")]
        clone[("Working clone<br/><i>of the record repository</i>")]
        nbclone[("Notebook clone<br/><i>of the person's own notes;<br/>optional, read-only</i>")]
        kernel -- "embeds queries and records" --> embed
        kernel -- "loads, validates" --> modules
        kernel -- "reads, writes, commits" --> clone
        kernel -- "reads, searches" --> nbclone
    end

    repo[("<b>Record repository</b><br/><i>private git; one per kernel instance;<br/>the trust boundary beneath the kernel</i>")]
    notes[("Notebook repository<br/><i>the person's own notes,<br/>written outside the system</i>")]
    idp["Identity layer"]
    tracker["Task tracker"]
    forge["Git forge"]
    consumer["Read-only consumer"]

    person --> cc
    plugin -- "MCP; /context, /instructions, /healthz" --> kernel
    person -. "browser" .-> idp
    idp -. "/view/, read-only (M3)" .-> kernel
    consumer -. "MCP, read tools only" .-> kernel
    clone -- "pull / push, secrets scanned at the host" --> repo
    nbclone -- "pull only" --> notes
    kernel -. "tracker adapter (M2)" .-> tracker
    kernel -. "forge adapter (M2)" .-> forge

    classDef person fill:#08427b,stroke:#052e56,color:#fff
    classDef container fill:#438dd5,stroke:#2e6295,color:#fff
    classDef store fill:#438dd5,stroke:#2e6295,color:#fff
    classDef ext fill:#999,stroke:#6b6b6b,color:#fff,stroke-dasharray:5 5
    class person person
    class cc,plugin,kernel,embed container
    class outbox,modules,clone,repo,nbclone,notes store
    class idp,tracker,forge,consumer ext
```

The diagram is deliberate about six things:

- **The plugin is thin.** It has three hooks and a few skills. `SessionStart` and `SubagentStart`
  always run; the `PreToolUse` guard always refuses writes to the saved instructions, and refuses
  writes to scratch only when a working-memory module is enabled. Installing it changes nothing in
  the assistant's global configuration except the plugin's own registration (spec §3.2).
- **The kernel is the only writer** to the working clone, and the clone is the only path to the
  repository. The claims runner is part of the kernel, so a claim result reaches the clone
  through the kernel's claim-result operation like any other change (spec §8.1).
- **The notebook is read-only.** The kernel pulls the person's own notes and never commits to
  them, so the one-writer rule still holds. The notes have no modules and so no audience of their
  own, which is why a consumer is refused them outright (spec §5, §11).
- **The embedding sidecar publishes nothing.** It is a separate container because the kernel is
  a static binary, and because the call to a model should cross a boundary that can be seen.
- **The identity layer belongs to the deployment**, not the system. The view has no
  authentication of its own and binds to loopback, so the deployment's identity layer is the only
  way to reach it (spec §10).
- **Two kernel instances with two repositories** is how a second trust domain, such as a memory
  shared with other people, would sit beside this one, with the plugin registering both. It is not
  drawn because it is not yet designed (spec §15).

## Level 3 — components of the kernel

This level shows what is inside the kernel, and which profile each component serves. The
components in the first group serve both profiles; those in the second serve only
`ratified-record` modules.

```mermaid
flowchart LR
    subgraph shared ["Shared by both profiles"]
        identity["<b>Identity</b><br/><i>who is calling: network layer or token;<br/>unidentified callers are refused</i>"]
        audience["<b>Audience filter</b><br/><i>per module, per consumer;<br/>hides, never removes</i>"]
        profiles["<b>Profiles</b><br/><i>working-memory · ratified-record;<br/>closed set; enforces each bundle</i>"]
        modloader["<b>Module loader</b><br/><i>validates manifests;<br/>refuses what it does not understand;<br/>checks budgets sum under the cap</i>"]
        schema["<b>Schema validation</b><br/><i>a write that does not match<br/>its module's kind is rejected</i>"]
        credref["<b>Credential refusal</b><br/><i>credential shapes rejected<br/>before git</i>"]
        store["<b>Store</b><br/><i>one file per record; frontmatter composed here;<br/>index maintained by the same write;<br/>pull → write → commit → push</i>"]
        retrieval["<b>Retrieval</b><br/><i>lexical + dense, fused;<br/>working-memory searched by default,<br/>ratified-record only when asked,<br/>the notebook when named</i>"]
        migrate["<b>Migration</b><br/><i>one-time: pre-module records<br/>retagged to the memory module</i>"]
    end

    subgraph ratified ["ratified-record only"]
        review["<b>Review</b><br/><i>question, verdict and answer into the commit;<br/>the only path that moves reviewed</i>"]
        claims["<b>Claims runner (M2)</b><br/><i>declared adapters, data-only arguments;<br/>pass · fail · no-evidence, each timestamped</i>"]
        adapters["<b>Adapters (M2)</b><br/><i>tracker · forge · date · manual;<br/>one implementation per backend</i>"]
        agenda["<b>Agenda</b><br/><i>fails by the claim's deadline, then behind, then drafts, then stale<br/>by priority and age, deferred behind, then onboarding; preferences last<br/>in each; then a budget item for instructions over budget; snoozes counted; open and no-evidence excluded</i>"]
        context["<b>Context renderer</b><br/><i>agenda line in reserved space,<br/>then module templates in priority order;<br/>2 KB hard cap; per-module budgets;<br/>overflow refused, never truncated</i>"]
        view["<b>View (M3)</b><br/><i>the same render as HTML, plus freshness,<br/>claim state, revision lines, snooze counts,<br/>manual fraction; read-only; loopback</i>"]
    end

    mcp[/"MCP endpoint<br/>search · read · list · write · delete · review · context ·<br/>modules (M2) · reflect (M2) · claims (M2) · claim_result (M2)"/]
    health[/"/healthz, and the kernel's four of the six<br/>silent failures /health reports"/]
    instr[/"GET /instructions<br/>the person's instructions, for the plugin's hooks"/]

    mcp --> identity --> audience
    instr --> identity
    audience --> retrieval
    audience --> store
    store --> schema --> credref
    modloader --> profiles
    profiles -. "governs" .-> store
    profiles -. "governs" .-> retrieval
    profiles -. "governs" .-> review
    claims -.-> adapters
    claims -. "results, through the store,<br/>never moving a goal's updated" .-> store
    agenda -.-> claims
    agenda --> store
    context --> agenda
    view -.-> context
    review --> store
    migrate --> store
    modloader --> health
    mcp -. "modules: the manifests, read-only" .-> modloader
    mcp -. "reflect: the gap by value, read-only" .-> agenda

    classDef comp fill:#85bbf0,stroke:#5d82a8,color:#000
    classDef iface fill:#fff,stroke:#5d82a8,color:#000
    class identity,audience,profiles,modloader,schema,credref,store,retrieval,migrate,review,claims,adapters,agenda,context,view comp
    class mcp,health,instr iface
```

For someone reading the diagram for the first time: a request enters at the MCP endpoint, is
identified, is filtered by audience, and only then reaches retrieval or the store. A write passes
schema validation and credential refusal before it becomes a commit. The profile of the record's
module decides what the write means: in a `working-memory` module it is complete, and in a
`ratified-record` module it leaves a draft that only a `review` can confirm. The claims runner
(M2) never sees a request; it runs on the deployment's interval, and its results enter the store
through the kernel's claim-result operation, so the kernel remains the only writer and a result
never makes a goal look revised. The working-memory freshness lint is not drawn, because it runs
in the plugin's `/health` skill rather than in the kernel (spec §4.2).

## Level 4 — the interview, as a dynamic diagram

The interview is the flow that makes the system more than a place to keep notes. The kernel makes
two decisions, what is due and what counts as reviewed, and the assistant holds the conversation
around them (spec §9). Keeping those two decisions in the kernel means the conversation can be as
loose as the person likes without anyone being able to mark a record confirmed that the person did
not confirm.

```mermaid
sequenceDiagram
    autonumber
    participant P as Person
    participant A as Assistant (plugin)
    participant K as Kernel
    participant S as Claims runner, inside the kernel (M2)
    participant T as Tracker / forge

    Note over S,T: on a schedule, independent of any session
    S->>T: run each non-manual claim
    T-->>S: evidence, or nothing
    S->>K: record the result through the claim-result operation, when a claim's state, detail or count changes

    Note over A,K: every session start
    A->>K: context, instructions
    K->>K: compute agenda: fails, behind, drafts, stale by priority and age, deferred behind, onboarding, then budget, with preferences last in each
    K-->>A: 2 KB block, first line is the top agenda item with its question and, if revised since last reviewed, its revision line, and the instructions beside it
    A-->>P: the assistant raises the line's question once, early, and not again that session

    Note over P,K: /interview, or the person picks up the first line
    A->>K: context, modules (M2)
    K-->>A: the agenda, and every enabled module's kinds, fields, lenses and intro
    A->>A: getting to know the person when the top agenda item is onboarding, else a check-in
    loop until the person says enough or stop (a "later" puts off one item, and the loop goes on)
        A->>P: a topic's lens, or the top agenda item, in the person's register
        P-->>A: an answer, as long and as wandering as they like
        A->>K: write a draft for each thing the answer holds, in any module, and a thread for what no module holds
        A->>K: delete a thread once a module that covers it holds a draft from it
        A->>P: the drafts, the interviewer's own inferences labelled, challenges where warranted
        P-->>A: confirmed / corrected / retired / later, in their own words
        A->>K: review(record, question, verdict, answer), once per draft
        K->>K: write question, verdict and answer into the commit, then move reviewed or count a snooze
    end

    A->>K: reflect (M2)
    K-->>A: per value, the goals that serve it and their claims' state, then goals serving no named value
    A-->>P: the gap, grouped by what the person said matters
```

The sequence makes four things visible that prose can hide. The scheduler and the session never
touch each other; they meet only in the store. The kernel decides what is due, and the assistant
decides how to ask about it. Drafts reach the store before anything is confirmed, so a stop in the
middle loses nothing and the next session's agenda opens on them. And `reviewed` moves at exactly
one step, on a call that carries the question, the verdict and the person's words, so the record's
history can be read as a list of things the person was asked and said.

## What is deliberately not drawn

- **A deployment diagram.** C4's fourth standard diagram places containers on infrastructure.
  That belongs to each deployment, and the specification deliberately describes none, so that it
  holds no one person's setup (spec §12). A deployment draws its own in the operator's notes.
- **The inside of a module.** A module is a manifest, templates and optional skills. It declares
  the adapters it uses but carries none, because an adapter is kernel code; so a module has no
  running code of its own to diagram. Its contract is spec §6.
- **The second kernel instance** for a shared memory across people (spec §15). When it is
  designed, it is a second copy of the container diagram with a different repository and the
  plugin's MCP registration pointing at both.

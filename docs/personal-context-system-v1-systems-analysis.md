# The personal context system as a system — analysis of v1 with the full method

**Written 2026-09-26 against the spec as of that date, with the full SystemsThinking method;
supersedes the same-day analysis run without the workflow files; nothing built.**

The spec answers LifeOS's failures by moving enforcement out of the model and into a server that
refuses, validates and runs checks (§3.3, §3.4). The full method confirms that this is the right
paradigm and that the design applies it to claims and not to the loop it calls "the mechanism that
keeps the record true" (§9): the interview is entered on a cue addressed to the model, its agenda is
chosen by the model, and the field the whole freshness signal runs on, `reviewed`, is bumped by a
write the kernel cannot tell apart from any other. That is the axiom of §1 delegated, at its three
load-bearing points, to the party §2.2 documents faking. Around that centre the method finds five
further structures the spec does not name: claim results that cannot say "no evidence" or "adapter
down", so the interview can open with a false accusation; a scope model with no audience dimension,
so a read-only consumer reads health and finance the day M4 lands; a 2 KB block shared by modules
with no allocation rule, whose naive reading is the silent truncation §2.2 is built against; a
frictionless path to lowering a goal with nothing that shows the lowering; and a stated goal, the gap
reflected by value (§1), that no milestone delivers. The highest-leverage change is one rule: the
kernel owns the interview's two decisions — what to ask first, rendered into every session, and what
counts as reviewed, through a distinct operation that carries the person's answer.

Method: the skill's five workflows in the order their own Integration sections chain them — Iceberg
on the failure modes of §2.2, which feeds ConceptMap and CausalLoop; ConceptMap for the entities
the loops run over; CausalLoop for four loops the design creates, which feeds FindArchetype;
FindArchetype against them, which exits to FindLeverage by Meadows' ranking. Each workflow's
output block is filled with its field labels kept and its heading glyphs dropped; 3.4 is rendered
as a table because its loop is 3.1's B1 and the table is where the finding lives. Findings
are labelled inline: (a) a structural problem in the design as written, (b) a risk that depends on
deployment, (c) something the spec already handles. Unknowns are marked; no number here is invented.

## 1. Iceberg: the failure modes the spec reacts to

```
ICEBERG ANALYSIS: LifeOS v7.40.4 as installed 2026-09-24 (spec §2.2)

EVENTS (Layer 1):
- Update path merges hooks only; settings hardening never reaches an existing install (#1860)
- A reordered array is silently pinned; later additions are dropped (#2162)
- `lifeos -m <shortcut>` replaces `~/.claude/.mcp.json` with an empty object, no backup, exit 0 (#2156)
- The documented system/user settings split is never established by the installer (#2086)
- The safety hook auto-allows MCP calls the operator listed under `permissions.ask` (#2087)
- `auto` mode steers writes through Bash, silencing four PostToolUse hooks (#2079)
- Assistants "pretend to run through the algorithm" and edit verification sections without doing
  the work (discussion #1312)
- Three hours and a WSL migration to onboard a moderately technical user (#922)

PATTERN (Layer 2):
- Time window: one install, one version; eight issue and discussion numbers, dates not checked
- Shape: escalating accretion with silent failure. Two shapes, one rhythm: every observed lapse
  is answered by another instruction to the model (74 hooks, 57 skills, a 25 KB constitution),
  and the mechanisms added fail without a signal (exit 0, silenced hooks, dropped entries)
- Trigger conditions: any failure whose remedy is "tell the model more"; any configuration the
  model is permitted to edit

STRUCTURE (Layer 3):
- Primary generator: enforcement lives inside the thing it enforces. The model reads the rule,
  the model runs the ceremony, the model is auto-approved to edit the hooks and the system
  prompt. A rule the enforced party can skip, fake or rewrite is advisory, whatever it is called
- Contributing structures: (1) a reinforcing loop, failure → new hook or doctrine → more
  context per session → more to skip → more failure; (2) no ownership boundary — nothing sits
  outside the model to observe it, so the delay from a faked step to its detection is the time
  until a user notices in a discussion thread; (3) everything ships to everyone, so every
  user carries every module's failure surface
- Test: remove any one hook or issue and leave the structure — another appears of the same
  shape. Confirmed by the issue list itself: four independent silent-failure bugs in one install

MENTAL MODEL (Layer 4):
- Belief that makes the structure feel correct: "the constitutional rule is the boundary and
  regex layers are not worth having" (Security — Minimal v2, quoted in §2.2); beneath it,
  "more articulation produces more compliance"
- Who holds it: the LifeOS author, stated in the security document; the harness encodes it
- What evidence would shift it: #1312, a user reporting the ceremony faked — which is the
  evidence §2.2 cites and §3.4 is built on

INTERVENTION CANDIDATES:
- Event-layer patch: fix the six issues (LifeOS's own path; consents to recurrence)
- Structural fix: put every enforcement in a process the model cannot skip or edit; cap what
  the model reads; ship modules opt-in
- Mental-model shift: "a check that runs cannot be faked; a hook that nags can" (§2.2) —
  the server is the boundary (§3.3), never a warning (§3.4)

RECOMMENDED: the mental-model shift, and the spec has made it. This section's job is to test
whether the spec's structure follows its own belief everywhere.
```

Walking back up through the spec, layer by layer:

- Mental model: adopted in full (§3.3, §3.4, §2.2's last bullet). (c)
- Structure, the boundary: the kernel is the only writer, validates writes against schema, enforces
  scope on read, refuses credential shapes, runs adapters on a schedule; the plugin has two hooks
  and no permission changes (§3.2, §4, §11). (c) The structure survives in three places, though:
  the interview (§9) is a skill, which is instructions to the model — steps 3, 4 and 6 (open with
  the sharpest contradiction, never ask what is on file, reflect back) are exactly the kind of
  ceremony #1312 reports faked; the ratification that bumps `reviewed` (§9.5) is a write the model
  makes; and modules may contribute skills the plugin loads (§4.2, §6), an uncapped path into the
  session beside the capped block. (a)
- Structure, silent failure: the pattern of §2.2 is that a mechanism fails and reports success. The
  spec has four paths of its own with no reporting rule: the 2 KB cap when modules exceed it (§10 —
  the allocation rule is not stated, and "in priority order ... hard-capped" reads as truncation);
  the outbox drained at `SessionStart` when a queued write fails validation (§4.2, §15 — where the
  rejection goes is not said); the scheduler dying while old claim results stay on file with their
  old timestamps (§8.1, §15); and an adapter that errors or finds no evidence, stored as `fail`
  (§8.1, treated in §3.2 below). `/health` (§4.2) reports none of the four. (a)
- Events: none yet; the spec is unbuilt.

## 2. Concept map: paths into and out of the record

Included because the loops below run over trust boundaries, and a map with labelled propositions
shows where two paths share a boundary that the prose keeps apart.

```
CONCEPT MAP: the record's inflows, outflows and who is trusted on each

FOCUS QUESTION: Through which paths does content enter or leave the record, and who is
trusted at each?

CONCEPTS (general → specific):
- Person; Assistant session; Read-only consumer; Operator
- Kernel; Plugin (SessionStart, PreToolUse, /interview, /done, /health); Scheduler
- Record (private git repository); Git host; Module; Kind; Claim; Adapter; Evidence source
- Context block; View; Outbox; Module skill; `reviewed`; Scope; Identity

HIERARCHY:
  Record
    ├─ Module → Kind → record file (`reviewed`, `updated`, `id`)
    │             └─ Claim → Adapter → Evidence source
    ├─ Context block (render) → Assistant session
    └─ View (render) → Person, behind the deployment's identity layer

PROPOSITIONS (concept → [link] → concept):
- Kernel → [is the only writer of] → Record (§5, §11)
- Kernel → [validates against] → Module schema (§5, §6)
- Interview skill → [writes ratified corrections through] → Kernel (§9.5)
- Assistant session → [adds to the record during a session through] → Kernel (§1, §4.2)
- Kernel → [bumps] → `reviewed` on interview writes (§9.5)
- Kernel → [refuses] → credential shapes (§11); Git host → [scans pushes for] → secrets (§11)
- Scheduler → [runs] → Adapters; results → [are stored on] → the goal (§8.1)
- Adapter → [reads from] → Evidence source using deployment credentials (§6)
- Kernel → [renders] → Context block, hard-capped at 2 KB (§10); Plugin → [injects at] → SessionStart
- Kernel → [renders] → View, read-only, no POST routes (§10, §11)
- Identity → [selects] → Scope; Scope → [filters] → every read (§5, §11)
- Read-only consumer → [receives] → read tools only (§11)
- Outbox → [is drained at] → SessionStart (§4.2)
- Module → [contributes] → Module skill; Plugin → [loads] → Module skill (§4.2, §6)
- /done → [produces] → a done-statement in the working repository, outside the record (§8.2)

CROSS-LINKS:
- Interview write and session write → [arrive through the same] → kernel write. The spec says
  the kernel bumps `reviewed` on the first and not the second, and names no way for the
  kernel to know which it is receiving.
- Read-only consumer → [resolves to] → `global` scope → [which contains] → identity, health
  and finance records. The fail-closed rule is in the kernel's documentation (an unresolved caller sees `global`
  and `project/`), carried in by §4.1 and §5 "unchanged"; the assumption is that health and
  finance are `global`-scoped, which nothing in §7 contradicts. Scope is by machine and project
  (§5); it has no audience dimension, so "read-only" bounds integrity and not confidentiality.
- Evidence source → [must be shaped by] → Person: tracker claims join on labels, forge claims
  on repository names (§8.1). The remembering §3.1 removes from the record reappears here.
- Scheduler → [writes results into] → Record → [renders into] → Context block. The check that
  runs writes through the one writer, so a sibling scheduler (§15) is a second writer unless
  it goes through the kernel.
- Outbox → [rejected at drain] → no named destination; the person's ratified correction is
  the content most likely to be queued offline.
- Module skill → [enters] → session context uncapped, beside the block the cap governs.

KEY INSIGHTS:
- Two inflows share a boundary the spec treats as one; the field meant to separate them has
  no mechanism behind it.
- The confidentiality of health and finance depends on a scope model built for machines.
- Every "the person does nothing" claim has a relocated remembering behind it — to the
  evidence source, the operator, or the model.
```

## 3. Causal loops

Delays, stated once: `freshness_days` is 90 or 365 (§6); the adapter period is "a schedule",
unspecified (§8.1, §15); the time from a record going stale or a claim going false to `/interview`
being invoked is unbounded; from ratification to the next render is immediate.

### 3.1 Interview, record freshness, context quality

```
CAUSAL LOOP DIAGRAM: what keeps the record true after novelty fades

QUESTION: What keeps the record true once sessions are routine, and what happens when
nobody invokes /interview?

VARIABLES:
- Stale records (count past their kind's freshness_days)
- Interview rate
- Cue strength (how often the "N records stale; interview due" line becomes an interview)
- Context truth (fraction of the block still true of the person)
- Session fit (how well sessions serve the person)
- Trust in the record
- Sessions with the record loaded
- Review debt (records to work through per interview)

ARROWS (source → target, polarity, delay?):
- Calendar →(+) Stale records (delay: freshness_days)
- Interview rate →(−) Stale records
- Stale records →(+) Cue line rendered (kernel-computed, §9)
- Cue line →(+) Interview rate — advisory; the model must voice it or the person must
  remember; strength unknown; the arrow §3.4 forbids relying on
- Stale records →(−) Context truth
- Context truth →(+) Session fit
- Session fit →(+) Trust in the record
- Trust →(+) Sessions with the record loaded (soft)
- Sessions →(+) Cue sightings →(+) Interview rate
- Session fit →(−) Friction episodes; Friction →(+) Interview rate (delay: the person has
  to notice the assistant is wrong about them and ask)
- Stale records →(+) Review debt →(−) Interview rate (a long interview is deferred;
  damped by §9.4's clean exits, which make a partial interview legal)
- Interview rate →(+) Context truth (ratified corrections, §9.5)

LOOPS:
- B1 "Cue to review": Stale → Cue → Interview → Stale. One minus: balancing. Every link is
  mechanical except Cue → Interview.
- B2 "Repair after harm": Stale → Truth → Fit → Friction → Interview → Stale. Three minus:
  balancing, delayed; the damage arrives before the repair.
- R1 "Quiet decay": Stale → Truth → Fit → Trust → Sessions → Sightings → Interview → Stale.
  Two minus: reinforcing. Run the other way it is the virtuous loop the spec builds for
  (§9.4's "never ask what is on file" and clean exits protect it).
- R2 "Review debt": Stale → Debt → Interview → Stale. Two minus: reinforcing.

DOMINANT LOOP: B1, while the system is new and the person invokes the interview unprompted.
EMERGING DOMINANT LOOP: R1 and R2. The first staleness arrives freshness_days after the first
interview — 90 days at the shortest — which is past the novelty in which B1 was running on
habit rather than on the cue.

INTERVENTION ANALYSIS:
- Proposed (by the spec): the cue line, and no daemon, chip or notification (§9).
- Intended effect: B1 closes without a nag.
- Unintended: nothing pushes back; the problem is B1's gain, which rests on one advisory link.
- Recommended: raise B1's gain without a notification, by making staleness change what the
  kernel renders — the due item itself, with its kind's interview prompt, as the first line
  of the block — and by making the ratification a kernel operation. Section 5.
```

Structural, (a): the loop that keeps the record true is built from the mechanism §3.4 forbids.
Already handled, (c): the partial interview (§9.4) damps R2; "never ask what is on file" feeds R1's
good direction.

### 3.2 Claims, adapters, goal honesty

```
CAUSAL LOOP DIAGRAM: whether claims keep goals honest

QUESTION: Do claims keep goals honest, or do goals become what the adapters can count?

VARIABLES:
- Adapter coverage (goals whose claims something other than manual can check)
- Runnable claims
- Measured contradictions (claims that went false against real evidence)
- False contradictions (fail stored when evidence was absent or the adapter errored)
- Opening credibility (the person's belief that the interview's opening is true)
- Goal honesty (correspondence between stated goals and what the person values)
- Claim softening (edits that lower min, move since, or retire the claim)
- Measurability bias (goals phrased as what tracker and forge can count)

ARROWS:
- Adapter coverage →(+) Runnable claims
- Runnable claims →(+) Measured contradictions (delay: adapter period, unknown)
- Measured contradictions →(+) Opening sharpness (§9.3)
- Opening sharpness →(+) Goal honesty, scaled by Opening credibility
- Goal honesty →(−) Measured contradictions (goals met, or reframed truthfully)
- Measured contradictions →(+) Discomfort →(+) Claim softening →(−) Measured contradictions
- Claim softening →(−) Goal honesty
- Runnable claims →(+) Measurability bias →(−) Goal honesty
- Measurability bias →(+) Adapter coverage (goals migrate to where adapters are)
- Evidence-source shaping lapses (labels, repo names) →(+) False contradictions
- Adapter error (token expired, forge down) →(+) False contradictions — because §8.1 stores
  pass/fail only
- False contradictions →(−) Opening credibility →(−) Goal honesty; and →(−) Interview rate
  (feeds 3.1's B1: a person greeted with a wrong accusation says "later")
- Manual claims →(+) Measured contradictions only at an interview (§8.1; recorded per M4);
  no unattended discovery for health or finance (§7)

LOOPS:
- B3 "Evidence confronts": Runnable claims → Contradictions → Sharp opening → Honesty →
  Contradictions. Balancing toward truth; the best loop in the design.
- B4 "Soften the claim": Contradictions → Discomfort → Softening → Contradictions.
  Balancing toward comfort; the drifting-goals loop.
- R3 "Count what counts": Runnable claims → Measurability bias → Adapter coverage →
  Runnable claims. Reinforcing; attention and articulation both tilt to work.
- R4 "Cry wolf": False contradictions → Credibility ↓ → openings dismissed → Softening
  cheaper and Interview rate ↓ → ... Reinforcing decay of the interview's authority.

DOMINANT LOOP: B3, in a deployment with tracker and forge wired (§13's third clause).
EMERGING DOMINANT LOOP: R3 as goals accrete claims where adapters are; R4 the first time a
deployment credential expires — §6 puts tokens in the environment and nothing in the spec
handles their expiry or distinguishes it from a false claim.

INTERVENTION ANALYSIS:
- Proposed (by the spec): claims with named adapters, run on a schedule, opening the interview.
- Intended: B3.
- Unintended: R3 starves health and finance of the one thing that makes an interview sharp;
  R4 is armed by the binary result model; B4 has a frictionless path and no visibility.
- Recommended: three-state results (pass, fail, no-evidence-or-error) with the result's own
  age, and §9.3 ranks only real fails as contradictions; manual fails rank equally; claim
  and goal revisions render as revisions.
```

Structural, (a): the binary result (§8.1) plus "open with the sharpest contradiction" (§9.3) means
an expired token opens the interview with a false accusation; a stale pass is also treated as a
pass, since results carry a timestamp and nothing reads it. Structural, (a): B4 has no visibility —
the view shows freshness and claim state (§10), not a goal's revision history, though stable ids
(§5) and git make it free to show. Structural in v1, (a), relieved by deployment, (b): R3, because
three of the four core modules have only `manual` (§7). Deployment, (b): evidence-source shaping
(labels) is the person's remembering, relocated.

### 3.3 The 2 KB cap and module pressure

```
CAUSAL LOOP DIAGRAM: what a hard cap generates as modules grow

QUESTION: What does a fixed 2 KB block generate as modules and records accumulate?

VARIABLES:
- Enabled modules; Records per module
- Wanted bytes (what every template would render)
- Truncation (bytes cut) — allocation rule unknown (§10)
- Priority contention (modules claiming a lower priority number, §6)
- Template computation (departure from "select and format", §6)
- Pressure to raise the cap
- Reliance on on-demand search for what the block omits
- Context quality

ARROWS:
- Enabled modules →(+) Wanted bytes; Records per module →(+) Wanted bytes
- Wanted bytes →(+) Truncation
- Truncation →(−) Context quality, silently if truncation is by priority order
- Truncation →(+) Priority contention
- Truncation →(+) Template computation (templates squeeze) →(−) the logic-light rule holds
- Truncation →(+) Pressure to raise the cap →(−) Truncation, if the cap moves. The spec
  anchors the cap as a design constraint (§10) and the deployment note says a change is a
  spec change, not configuration
- Truncation →(+) Reliance on search →(+) Context quality, only if the block tells the
  assistant what to search for

LOOPS:
- B5 "Say less": Wanted → Truncation → Templates compress → Wanted. Balancing; intended.
- R5 "Claim the top": Truncation → Priority contention → earlier render for one module →
  Truncation for another → Contention. Reinforcing across module authors: a commons.
- B6 "Raise the cap": held off by the anchor rule; if it ever runs it is drifting goals.

DOMINANT LOOP: B5, while the four core modules fit.
EMERGING DOMINANT LOOP: R5 the moment a fifth module is enabled (M5 adds two in the
reference deployment, one of which its own note says "would like more").

INTERVENTION ANALYSIS:
- Proposed (by the spec): a hard cap, priority order, logic-light templates.
- Intended: B5.
- Unintended: R5, and a silent truncation path of the §2.2 kind.
- Recommended: a per-module byte budget in the manifest, the sum validated on load;
  enabling a module that would overflow is refused (a denial, §3.4); any truncation that
  still happens is a `/health` failure and shows in the view. §10's own word "share"
  implies the budget exists; the spec has only to say so.
```

Structural, (a): the allocation rule is undefined and the naive reading is silent truncation of
the lowest-priority module. Already handled, (c): the cap itself, anchored as a constraint; 2 KB
is a parameter (LP 12) but one that is a threshold forcing structure, so the number is the right
kind of parameter — the missing piece is the rule, not the size. Structural, (a), small: §4.3
binds "enabled" to interviewed, summarised and verified as one switch, so a module wanted in
search and interview but not in the block has no setting.

### 3.4 The axiom, and who actually remembers

§1's axiom: "without the person having to remember to maintain it." §3.1: where a step would rely
on the person, it is automated or backstopped. The loop here is the same as 3.1's B1; the useful
artefact is the table of where each remembering went.

| what keeps the record true | mechanism in the spec | who remembers now | §3.1 honoured? |
|---|---|---|---|
| computing freshness | kernel, from `reviewed` and `freshness_days` (§9) | the kernel | yes |
| turning "interview due" into an interview | one line in the model's context (§9) | the model, then the person | no — delegated to the party §3.3 and §3.4 say not to rely on |
| deciding what to ask first | the skill (§9.3) | the model | no |
| bumping `reviewed` only for ratified content | "the kernel bumps `reviewed`" on interview writes (§9.5); no distinguishing mechanism named | the model's discipline | no |
| running claims | adapters on a schedule (§8.1) | the kernel or a sibling (§15) | yes while the scheduler lives; nothing checks that it does |
| evidence existing for a claim | tracker labels, repository names (§8.1) | the person, in the evidence source | relocated, not removed |
| adapter credentials | the deployment's environment (§6) | the operator | relocated; expiry unhandled |
| answering manual claims | asked at interview (§8.1) | the person, at an interview that must first happen | depends on the second row |
| the record's persistence | the private git remote as the only persistence (§5) | the operator; backup is unmentioned | relocated |
| what fits in 2 KB | module templates (§10) | module authors | relocated, no rule |
| the outbox draining correctly | drained at `SessionStart` (§4.2); validation at drain open (§15) | nobody, if a rejection is silent | unknown |

The axiom is honoured where the kernel computes and where adapters run. It is delegated to the
model at the three rows that decide whether an interview happens and whether its result is real.
Every other row is a relocation, which is legitimate as long as the spec says so; today it says the
person does nothing.

## 4. Archetypes

Each match below passed the workflow's negative test — the intervention addresses the loop, not a
symptom, accounts for the delay, and has a record of helping in a like case — or is marked as merged
or relabelled for failing it.

**Shifting the burden — the spec's cure, and its residue.** Symptomatic solution: instructions to
the model (LifeOS). Fundamental: enforcement in a process the model cannot skip. The spec makes the
fundamental move for scope, schema, secrets and claims (§3.3, §3.5, §11). The residue is 3.1's B1 and
the three "no" rows of the axiom table: the interview's initiation, agenda and ratification are still
the symptomatic form. Intervention: apply the fundamental solution to §9 — the kernel computes the
agenda and owns `reviewed`. Negative test: it removes the model from B1's critical links; the delay
becomes "the next session"; the record of it helping is the claims mechanism itself, which is the
same move and the part of the design nobody doubts. (a)

**Policy resistance — ratification at no cost, and the bump without a question.** "Still right?"
answered "yes" bumps `reviewed` (§9.5) and clears the count; freshness then measures how recently the
person nodded. The full method adds the sharper case: the kernel bumps `reviewed` on interview writes
and has no way to know a write is one, so the model can bump it without having asked — #1312's
pattern arriving at the one field B1 runs on. Intervention (canonical: redesign the measurement so it
captures the quality, not a proxy): `reviewed` moves only through a distinct `review` operation that
carries the person's answer verbatim into the commit, so a bump without a question is visible in
history and a plain `write` never bumps it. (a)

**Success to the successful — evidence flows to work.** 3.2's R3. Tracker and forge run unattended;
health and finance have `manual` (§7), which can only replay at an interview what the person already
said. The sharpest contradiction (§9.3) will be a work goal for as long as the adapter set stands,
and a system that rewards checkable goals teaches the person to write goals in the shape it can
count. Intervention (canonical: allocate independently, not competitively): the agenda ranks a manual
fail equal to an adapter fail, counts a manual claim unanswered past its goal's freshness as due, and
renders the manual fraction in the view. (a) in v1; (b) once a deployment writes adapters.

**Growth and underinvestment — adapters are the capacity.** The earlier analysis carried this
twice, once as shifting the burden ("manual is the symptomatic adapter") and once here; the first
fails the negative test — `manual` removes the pressure to write an adapter but atrophies no
capability — so the two are merged. The leading indicator the archetype asks for is the manual
fraction of claims, which the kernel already knows. (b), with a cheap (a) fix.

**Drifting goals — the interview creates the path of least resistance.** 3.2's B4. When the opening
is "G3's claim is false", two of the person's three exits are edits (§9.5 writes any ratified edit),
and lowering `min` or moving `since` clears the contradiction without touching the title. Git keeps
every revision (§5) and nothing renders it. Intervention (canonical: make drift visible; separate
revising the goal from reviewing performance): the goal's revision line, from history, shown in the
interview and the view; "revise" and "review" as distinct kernel operations. (a)

**Tragedy of the commons — the block.** 3.3's R5. `priority` is self-declared (§6) and the block is
one shared 2 KB (§10). Intervention (canonical: quota): a per-module budget validated on load. (a)

**Policy resistance — pressure relocates to skills.** Relabelled from the earlier analysis's "fixes
that fail": there is no delayed consequence that worsens the original problem, only pressure the cap
shuts out of the block finding the uncapped path (module skills, §4.2, §6). A skill body loads on
invocation, so this is a leak, not a highway; one sentence in §10 saying what a module's skills may
add to a session is enough. (a), small.

Considered and not matched: escalation (no second actor); limits to growth (the record's growth is
bounded by search, not by the block, and retrieval scales); accidental adversaries (the person and
the assistant have no hidden loop working against each other in the design as written); fixes that
fail for the cue line (no delayed side effect, only insufficient gain).

## 5. Leverage

```
LEVERAGE POINT ANALYSIS: the personal context system v1

SYSTEM GOAL (implicit): read from where the spec spends its words and what §13 tests — a
record that is fresh, schema-valid, scoped, secret-free, one-writer, rendered into a block
that fits. The stated goal (§1) is the gap between stated values and evidence, reflected
back. The stated goal has no mechanism: values are stored and not wired (§15), and M1–M6
accept the system without them. Until wired, the system optimises for its proxies.
DESIRED BEHAVIOUR: the record stays true without the person remembering; the interview
opens with evidence that is real; the gap is reflected by value.

CANDIDATE INTERVENTIONS:
- [A] The kernel computes the interview agenda (real contradictions first, manual fails
      equal, then stale by priority and age) and renders the top item, with its kind's
      prompt, as the first line of every block; `reviewed` moves only through a distinct
      `review` operation carrying the person's answer. Level 5 (rule), with 6 and 9 inside it.
- [B] Wire values: a `serves` field on `goal`; §9.6 groups the reflection by value.
      Level 3 (goal). Named in §15.
- [C] Three-state claim results with their own age; §9.3 ranks only real fails.
      Level 6 (information flow) plus a rule.
- [D] Per-module byte budget in the manifest, validated on load. Level 5.
- [E] Scope gains an audience dimension: a module is readable per consumer, not only per
      deployment; health and finance default to the person's own sessions. Level 5.
- [F] Revision lines for goals and claims, from git, in the interview and the view. Level 6.
- [G] Raise the cap, or change freshness_days. Level 12. Not recommended.
- [H] A daemon or notification cadence. Level 9. The spec refuses it (§9) and should keep to
      that refusal.

HIGHEST FEASIBLE LEVERAGE: [A]
- Meadows level: 5, a rule about who decides the agenda and what counts as reviewed
- Why feasible: nothing is built; it lands in M1 (the rendered line, `review`) and M2 (the
  evidence branch); it is stateless — computed from fields the record already has
- Why highest: [B] is a higher level and should be done, but it changes what the system
  says; [A] changes whether the loop that produces anything to say keeps running. Without
  [A], [B]'s reflection is delivered only when someone remembers to ask. Level 2 is already
  the spec's own paradigm (§3.3, §3.4); [A] is that paradigm applied to §9.

BUNDLED LOW-LEVEL TACTICAL INTERVENTION: [C] and [D] — cheap, both denials or checks in the
sense of §3.4, and each closes a silent-failure path found in §1.

EXPECTED EFFECTS:
- First-order: every session start carries the one due item; `reviewed` becomes attributable
  to a question and an answer.
- Second-order: `/interview` shrinks to "ask the top item, call `review`" — the LifeOS lesson
  applied to the skill itself; the model's discretion is confined to phrasing.
- Third-order: freshness measures ratification rather than writes; drift becomes visible
  because revise and review are distinct operations in history; the empty-record interview
  of §15 becomes the same mechanism with an agenda of "nothing on file".

RESISTANCE TO EXPECT:
- Policy resistance from the person: a question at every session start is friction, and
  "later" every time is the workaround. Mitigation: one item only; only real contradictions
  and records past a hard staleness; "later" recorded in the record with a count the view
  shows, so the snooze is itself visible.
- From the model: it may still not voice the injected line. Residual, and the smallest
  surface the design can leave — the view (§10) shows the same state to the person
  independently.
- Paradigm clash: none; it is §3.3 and §3.4.

MEASURE OF SUCCESS: §13 gains the clause it lacks today — a record left past its
freshness_days while sessions continue is surfaced without `/interview` being invoked —
and the record's history shows `reviewed` bumps only on `review` commits.
TIMELINE: M1 for the rendered line and `review`; M2 for the evidence branch and three-state
results; re-evaluate at M4, when health and finance join with `manual` only and R3 can be
observed against a real record.
```

## 6. Findings by class

Structural problems in the design as written, (a):

1. The freshness loop runs on an advisory cue to the model (§9), which §3.4 forbids. Sections 1,
   3.1, 3.4.
2. `reviewed` is bumped on interview writes (§9.5) with no mechanism that tells the kernel a write
   is one; the model can bump it without asking. Sections 2, 4.
3. Claim results are binary (§8.1); an adapter error or absent evidence opens the interview as a
   contradiction (§9.3); a stale pass is a pass. Section 3.2.
4. Scope has no audience dimension (§5, §11); a read-only consumer resolving to `global` (the
   kernel's documented fail-closed rule) reads health and finance once M4 lands, assuming they are
   `global`-scoped. Section 2.
5. The 2 KB allocation rule is undefined (§10); the naive reading is silent truncation. Section 3.3.
6. Goal and claim revisions are invisible; the interview reads the goal as it stands (§9.6, §10).
   Sections 3.2, 4.
7. The evidence opening is confined, in v1, to modules with adapters; health and finance can never
   produce it (§7, §9.3). Section 3.2.
8. Four silent-failure paths — cap overflow, outbox rejection at drain, scheduler death, adapter
   error — and `/health` (§4.2) reports none of them. Section 1.
9. A sibling scheduler that stores results is a second writer unless it writes through the kernel
   (§8.1, §11, §15).
10. The stated goal (§1) has no mechanism until values are wired (§15). Section 5.
11. Smaller: `enabled` is one switch for interview, summary and verification (§4.3); the
    unstructured tier and disabled modules stay searchable with no freshness in ranking (§4.3, §5 —
    the server's own `delete` rationale is the evidence that stale records compete); module skills
    are an uncapped path into the session (§4.2, §6); `/done` says its check blocks run under "the
    working repository's tooling" (§8.2), which the product does not ship, so for a second person
    they are documentation.

Risks that depend on deployment, (b):

1. Evidence-source shaping — labels, repository names — is the person's remembering, relocated
   (§8.1). A lapse looks like a false claim until finding 3 is fixed.
2. Adapter credentials live in the environment (§6); their expiry is the R4 trigger.
3. The record's only persistence is its git remote (§5); backup is a deployment property the spec
   does not require.
4. Manual-only health and finance (§7) stay honest only if a deployment writes adapters; the manual
   fraction is the leading indicator.
5. Habituation: a person reliably behind on one goal is reliably greeted with it; whether a rotation
   rule is needed is unknown until a deployment runs.
6. The `PreToolUse` deny on the built-in memory path (§4.2) invites the model to write somewhere not
   denied.
7. A hand edit pushed to the record skips §5's validation and is adopted at the kernel's next reset.

Already handled, (c), extending rather than restating the earlier list: the mental model of §2.2
adopted in full; one writer; scope on read; no shell in claims and nothing executing from the
record; read-only consumers for integrity; the interview rule and never-ask-what-is-on-file; no
stop-time gate; the cap as a constraint, not a default; modules opt-in; `reviewed` distinct from
`updated`; stable ids; clean interview exits, which damp R2; §15's candour about the empty-record
interview and the unwired values; and the deployment note's refusal to raise the cap for the
first module that wants more.

Unknowns: how the kernel shares 2 KB among modules; the adapter period; whether the interview runs
adapters live (§9.2) or reads stored results; what a summary template may select on — whether it
can prefer `reviewed` records; the magnitude of reflexive ratification; whether reflection moves the
person at all, which is the bet beneath the whole design and is stated nowhere as a bet.

## 7. Reconciliation with the earlier analysis

What survives the full method unchanged: the centre (B1 as one advisory line, R1's stale spiral, B2
as the best loop, R2's review debt); drifting goals; success to the successful; the commons on the
block; the single enable switch; two inflows, one render; claim results as writes and the
scheduler as a second writer; direct pushes; the leverage table's reading of LP 3, 9 and 8; the (c)
list.

Added by the method: the iceberg on §2.2 itself, whose pattern is silent failure as much as
accretion, and the four silent paths the spec carries (§6 (a) 8); binary claim results and the
false-accusation opening (3.2, R4); scope's missing audience dimension, surfaced by the concept
map's cross-links (2); the kernel's inability to tell a ratified write from any other, which turns
"two inflows" from a rendering gap into a ratification hole (4); the who-remembers table (3.4); a
concept map, which the earlier analysis omitted as adding nothing and which produced two of the
findings above.

Dropped or corrected: "fixes that fail — module skills" relabelled as policy resistance, failing
the archetype's delay test; "shifting the burden — manual adapter" merged into growth and
underinvestment, lacking an atrophy mechanism; the highest-leverage change upgraded from
"render the opening line" (LP 6 and 9) to "the kernel owns the agenda and `reviewed`" (LP 5), of
which the earlier pick is the first half — the second half closes the ratification hole the earlier
analysis did not see, and the two are one mechanism.

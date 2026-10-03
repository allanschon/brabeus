# Research spikes: decision models in the kernel

Written 2026-10-02. This is a set of questions to answer cheaply before anyone designs
anything. Nothing here is in the specification, and nothing here is planned for a milestone.

## Why these spikes exist

The kernel's only model today is the embedder, which measures similarity. A decision model
(Laya, Jev, or an older kind such as an NLI cross-encoder or a reranker) answers a different
kind of question: which of these options, how much, or how likely. That would let the kernel
do judgment-shaped work at write time and between sessions, when no session model is present.

The specification's authorship rule still applies to every idea below. The model may propose; the
person ratifies through `review`. A decision model must never write a ratified record, a claim
result, a claim state or a `reviewed` date. Claim states, the reflection's counts and the
`behind` pace rule stay derived from counts and dates, because the specification keeps them
checkable (§8.1, decision AO).

## What the research found about the candidate models

Most of this comes from secondary write-ups and vendor material, not from the models'
own documentation. Treat the numbers as directional, and re-measure on the record's own data
before relying on any of them.

- **Laya** (Convai Innovations; Apache 2.0; ModernBERT-large, about 421M parameters). Local, with
  p50 latency of 33 ms on a Tesla T4. It is a base to specialise, not a zero-shot model: about
  0.36 accuracy zero-shot against 0.32 for random, and 0.766 after fine-tuning on the typed-decisions
  benchmark. Fine-tuned results range from 0.99 (spam) through 0.76 (jailbreak), 0.66 (RAG passage
  relevance) and 0.52 (ten-way ticket routing). Choice questions degrade past about 20 options. The
  English checkpoint can report high confidence on scripts it cannot read.
- **Jev** (TypeSafe AI; closed; hosted API at $0.042 per million input tokens). Works out of
  the box and handles many options (0.870 on Banking77, reported). Latency is reported between 70 and
  500 ms. Its own limitations page names arithmetic and counting, date comparisons and multi-step
  reasoning as failures. Every published number is from the vendor, whose dataset and task
  definitions are not shared; its reference answers are the average of two frontier LLMs.
- **Neither is calibrated by guarantee.** Both are trained for calibration, but the sources
  disagree on the figures (Laya's expected calibration error appears as 0.081, 0.213 and others;
  Jev's as 0.144 and 0.246), probably from different configurations.
- **Older tools.** In a zero-shot benchmark (BTZSC), rerankers did best (Qwen3-Reranker-8B at macro
  F1 0.72, and the 0.6B already above NLI cross-encoders), embedding models gave the best accuracy
  for their speed (about 0.62), and NLI cross-encoders plateaued near 0.60. NLI models are trained
  for contradiction and entailment, so for that task they are the natural baseline.

## Rules for every spike

1. **Local only.** The embedding sidecar stays on loopback because every record's full text
   passes through it. A hosted model such as Jev would send ratified-record content to a third
   party, which the specification's posture does not allow. Jev can be tested only on the
   non-sensitive test corpus in `internal/retrieval/testdata`.
2. **Baseline first.** Each spike names the cheaper thing to beat. A model that does not beat it
   is a result, not a failure.
3. **Shadow mode.** The probe reads records and writes a report outside the record. It does not
   write, review, delete or change any claim.
4. **Time-box.** One day each, unless the spike says otherwise. Stop at the time-box and write
   down what was learned.
5. **A stated kill criterion.** Each spike says in advance what result ends it.

## Spike 0: label collection (do this first)

**Question.** Does the kernel already produce labeled examples as a side effect of `review`, and how
many?

**Why.** Laya needs fine-tuning data. Every draft that was confirmed, corrected, retired or
snoozed says whether a proposal was right. If the volume is in the tens, no model in this
document can be fine-tuned on it; if it is in the hundreds, some can.

**Probe.** Read the record's git history and count `review` commits by outcome and by kind. Check
what a review commit stores: the question, the original draft and the person's answer. Report
what a training pair would look like, and what is missing.

**Kill.** None. This spike sets the budget for every later one.

## Spike 1: supersession as a field, with no model

**Question.** If a record can name what it supersedes, does that remove most of the
contradiction problem without a model?

**Why.** Memory systems that handle contradictions well (Zep) do it structurally: the old fact
stops being valid and the new one points at it. Pure vector stores keep a corrected fact competing
with the original in results. This is deterministic and fits the kernel's design.

**Probe.** Read how `write` and `delete` treat a record that replaces another today. Count, in the
real record, how many pairs are in fact the same fact restated or reversed. Sketch the field
(`supersedes: <path>`), how `search` and `list` would hide a superseded record by default, and
what `review` would do with it. Write no code.

**Kill.** If fewer than a handful of such pairs exist in the real record, the field is not worth
its schema cost yet.

## Spike 2: relation check at write time

**Question.** When a new memory lands near an existing one, can a model separate duplicate,
supersedes, contradicts and unrelated?

**Why.** The write-time duplicate check scores the known pair at 0.853 with embeddinggemma. A
restatement and a reversal can both score high, so similarity alone cannot tell them apart.

**Probe.** Build a small labeled set of record pairs from the real record and the test corpus
(for example, "printer firmware pinned" against "printer firmware upgraded"), at least 40 pairs
across the four labels. Compare three methods on it: embedding similarity alone, an off-the-shelf
NLI cross-encoder (`tasksource/ModernBERT-large-nli`) and an LLM judge as the ceiling. Laya is
included only if Spike 0 shows enough labels to fine-tune it.

**Baseline to beat.** Embedding similarity with a tuned threshold.

**Kill.** If the NLI model does not separate contradiction from duplicate on the held-out
pairs better than similarity, drop it. The result then points back to Spike 1.

## Spike 3: automatic capture for working memory

**Question.** Can a small local classifier decide that an event is worth a note, and of which kind?

**Why.** The specification lists automatic capture as not designed and not planned for any milestone, naming a classifier
for tool calls and a pre-compaction summary. Today it depends on the model choosing to write.

**Probe.** Take about 100 events from a few past sessions (a tool call, a user correction, a
statement of a preference) and label each: worth a note or not, and which of `note`, `trap`,
`preference`, `thread`. Compare zero-shot embedding similarity to a one-line description of each
kind (using the sidecar already running) against a fine-tuned Laya and an LLM judge. Five labels is
inside Laya's range, so cardinality is not the concern.

**Baseline to beat.** Embedding similarity to the kind descriptions.

**Kill.** If the best local method misses more than a fifth of the events the person would have
wanted noted, it is not usable for automatic capture. It might still serve as a "you may want to
save this" nudge, and the report should say so.

## Spike 4: evidence suggestions for manual claims

**Question.** Can a model say whether recent activity shows progress on a manual claim?

**Why.** G1's photo count is entered by hand, and a goal about contact with siblings has no claim
at all. A scheduled pass could queue "evidence suggests 800, confirm?" for the next interview. The
claim stays manual and the person still answers `claim_result`.

**Probe.** Pick two real manual claims. For each, collect a month of candidate evidence (task
completions, commits, Immich activity if it is reachable) and label each item as showing progress
or not. Measure a reranker (`Qwen3-Reranker-0.6B`) and an NLI model against an LLM judge. Also
measure the simplest alternative: count the activity directly and show the number with no model.

**Baseline to beat.** Showing the raw activity count beside the question.

**Kill.** Relevance judgments are Laya's weakest reported area (0.66 fine-tuned), so expect a
negative result there. If the raw count does as well as any model, drop the idea. Counting is the
kind of task Jev's own page says it fails at.

## Spike 5: staleness prediction

**Question.** Does asking "is this record probably no longer true" rank records better than
age alone?

**Why.** Freshness is a clock rule today. The agenda orders failed claims by the goal's `by` date and
failure age.

**Probe.** Take the ratified records whose `reviewed` dates are old and the ones the person later
corrected or retired. Using only information available before each correction, check whether age,
newer related records or a model's judgment predicted the change. Spike 1 and Spike 2 results
feed this one.

**Baseline to beat.** Age since `reviewed`.

**Kill.** Run it only after Spike 0 has shown there are enough corrections to test against.
Without them there is no ground truth, and the spike stops.

## Spike 6: `serves` suggestions

**Question.** Can a model suggest which value an unlinked goal serves, or flag a goal that
contradicts its stated value?

**Why.** Goals that serve no value are listed by `reflect`, and the interview already raises them in
conversation, labelled as an inference. The gain is small.

**Probe.** Take the goals and values on file and compare an LLM judge's suggestions with the
person's own links, and with embedding similarity between goal text and value text. Choice
questions here stay under 20 options, which is inside Laya's range.

**Kill.** If the interview's conversational version already produces the same suggestions, stop.
This is the lowest priority of the nine.

## Spike 7: sensitivity and audience routing

**Question.** Can a classifier flag content as probably health, financial or family before it
reaches an `audience: any` module?

**Why.** G2 onboards another person onto the services, and consumers see only modules whose
manifest declares `audience: any`. A missed case here is worse than a false alarm.

**Probe.** This is a binary or small-choice decision, which is the task type where Laya's reported
results are strongest (0.98 to 0.99 on spam and phishing). Build a labeled set from the real record
and measure recall at a fixed false-alarm rate. Compare it with existing PII and secret detectors,
which the kernel's secrets boundary may already resemble.

**Constraint.** The check may only add a refusal or a warning. It must never widen audience or lift an
existing refusal.

**Kill.** If recall on the sensitive class is below the level at which the person would trust it as
the last check, it is at best a second layer behind the rule-based boundary.

## Spike 8: session-start relevance

**Question.** Would judging each working-memory hit's relevance to the current task improve what
the session sees over BM25 plus dense ranking alone?

**Probe.** Use `search_quality_test.go` and its corpus as the harness. Add a reranker stage
(`Qwen3-Reranker-0.6B`) over the top candidates and see whether the pinned paraphrases still reach
rank 1 and whether anything improves. Measure the added latency per query.

**Baseline to beat.** The current fused ranking, which already reaches both pinned paraphrases at
rank 1.

**Kill.** A reranker is the right tool class for this (Laya is not built for it), but the corpus is
small. If the existing ranking already passes every case, there is nothing to measure, and the spike
ends with that finding.

## Order

Spike 0 first, because it sets what is possible. Spike 1 next, since it needs no model and may
remove the reason for Spike 2. Then Spikes 2, 3 and 7. Spikes 4, 5, 6 and 8 follow the findings.

## Tasks

One task per spike, written to be copied into the tracker. Each is complete without this
document, because a reader of the task alone has to understand what is being asked and why the
result is not a design.

Every task shares the same status and the same deliverable:

- **Status: preliminary.** The data a task produces is exploratory, from a small sample, on
  one person's record. It is enough to judge whether an idea is feasible. It is not enough to
  design from, and no task builds anything to keep.
- **Deliverable: a feasibility finding and a decision**, written as a short note, with four parts:
  1. *Feasibility:* feasible, not feasible or inconclusive, with the numbers and the sample size
     behind it.
  2. *Baseline:* what the cheaper alternative named in the task scored on the same sample.
  3. *Decision:* invest further, park until a named condition holds, or drop, with the reason.
  4. *If invest:* what the next stage would need, such as data, hardware, or design questions, and
     a rough size.

  A negative or inconclusive result is a complete deliverable. A task is finished when the note is
  written, not when a model works.

### How many labeled examples does `review` produce?

> Preliminary. Feasibility and decision only.

Count `review` commits in the record's git history by outcome (confirmed, corrected, retired,
snoozed) and by kind, and check what each commit stores: the question, the original draft and the
person's answer. Report what one training pair would look like and what is missing from it.
The decision is whether any later task can fine-tune a model on this data, and at what volume.
Time-box: half a day. Depends on: nothing. Stop early if the record's history cannot be read.

### Can a `supersedes` field remove the contradiction problem without a model?

> Preliminary. Feasibility and decision only.

Read how `write` and `delete` treat a record that replaces another today. In the real record,
count the pairs that are the same fact restated or reversed. Sketch the field, how `search` and
`list` would hide a superseded record by default, and what `review` would do with it. Write no
code. The decision is whether the field is worth its schema cost now.
Time-box: one day. Depends on: nothing. Stop early if fewer than a handful of such pairs exist.

### Can a model tell duplicate, supersedes, contradicts and unrelated apart?

> Preliminary. Feasibility and decision only.

Build a labeled set of at least 40 record pairs from the real record and the test corpus.
Compare embedding similarity with a tuned threshold (the baseline), an off-the-shelf NLI
cross-encoder (`tasksource/ModernBERT-large-nli`) and an LLM judge as the ceiling. Include Laya only
if the label-count task shows enough labels to fine-tune it. The decision is whether a write-time relation check
is worth building, and with which model class.
Time-box: one day. Depends on: the `supersedes` task (so the pair set does not include cases the field would
remove). Stop early if the NLI model does not separate contradiction from duplicate better than
similarity does.

### Can a small local model decide what is worth noting, and of which kind?

> Preliminary. Feasibility and decision only.

Take about 100 events from past sessions and label each: worth a note or not, and which of
`note`, `trap`, `preference`, `thread`. Compare embedding similarity to a one-line description of
each kind (the baseline), a fine-tuned Laya and an LLM judge. The decision is whether automatic
capture is feasible locally, and if not, whether it works as a "you may want to save this" prompt.
Time-box: one day. Depends on: the label-count task. Stop early if the best local method misses more than a
fifth of the events the person wanted noted.

### Can a model say whether recent activity shows progress on a manual claim?

> Preliminary. Feasibility and decision only.

Pick two real manual claims. Collect a month of candidate evidence for each and label every item as
showing progress or not. Measure a reranker (`Qwen3-Reranker-0.6B`) and an NLI model against an LLM
judge, and against the baseline of showing the raw activity count beside the question with no
model. The decision is whether evidence suggestions are worth building. If the raw count does as
well, drop the model and consider only the count.
Time-box: one day. Depends on: nothing. Stop early if no activity source for the claims is
reachable.

### Does a staleness judgment beat age at predicting which records change?

> Preliminary. Feasibility and decision only.

Take ratified records the person later corrected or retired, and use only information available
before each correction to check whether age since `reviewed`, newer related records or a model's
judgment predicted the change. The decision is whether staleness prediction is worth building.
Time-box: one day. Depends on: the label-count, `supersedes` and relation-check tasks. Stop early if the label count shows too few corrections
to test against, because there is then no ground truth.

### Can a model suggest which value a goal serves?

> Preliminary. Feasibility and decision only.

Compare an LLM judge's suggestions with the person's own `serves` links and with embedding
similarity between goal text and value text. The decision is whether this adds anything beyond
what the interview already does in conversation. This is the lowest-priority task.
Time-box: half a day. Depends on: nothing. Stop early if the interview already produces the same
suggestions.

### Can a classifier flag sensitive content before it reaches an `audience: any` module?

> Preliminary. Feasibility and decision only.

Build a labeled set from the real record, marking health, financial and family content. Measure
recall at a fixed false-alarm rate for a fine-tuned Laya, existing PII and secret detectors, and the
kernel's current secrets boundary. The check may only add a refusal or a warning, and must never
widen audience or lift an existing refusal. The decision is whether a model adds a layer worth
having behind the rule-based boundary. Run the model locally only.
Time-box: one day. Depends on: the label-count task. Stop early if recall on the sensitive class is below what
the person would trust as a last check.

### Would a reranker improve what a session sees at start?

> Preliminary. Feasibility and decision only.

Use `search_quality_test.go` and its corpus as the harness. Add a reranker stage
(`Qwen3-Reranker-0.6B`) over the top candidates and check whether the pinned paraphrases still reach
rank 1 and whether anything improves. Measure added latency per query. The decision is whether a
reranker is worth adding. The corpus is small, so a result of "nothing to measure, the current
ranking passes every case" is a valid finding.
Time-box: half a day. Depends on: nothing. Stop early if the current fused ranking already passes
every case.

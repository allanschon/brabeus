---
name: interview
description: Use when the person wants to work through the agenda - the records the kernel is asking them to confirm, correct, retire or defer - or when the session's first line names one and they want to answer it now.
---

# Answer the agenda

The kernel keeps a record only the person can ratify. Its agenda is the list of
records to confirm — never reviewed, or past their freshness or a due date —
and gaps to fill where nothing of a wanted kind is on file; the questions it
asks are written in the module manifests. This skill asks them, one at a
time, and records the answer.

## The loop

1. Call `context` with no arguments. If `agenda` is absent, say "nothing due"
   and stop. Look at `agenda.reason`: `stale` and `draft` are both records to
   review (steps 2–4) — a `draft` was never reviewed, so it is confirmed or
   corrected the same as a `stale` one; `onboarding` is a question — nothing
   of that kind is on file yet — and the answer becomes a new record (step 4b).
2. Ask the person `agenda.question` verbatim, naming the record
   (`agenda.module/agenda.kind`, its `id` or `name`) and, when present, the
   revision line and the snooze count ("you have put this off N times").
   Show the record if they ask: `read` with `agenda.path`.
3. Wait for their answer. For a `stale` or `draft` item, map it to a verdict:
   - **confirmed** — it still holds (`stale`), or is right as written (`draft`).
     No body, no fields.
   - **corrected** — it changed. Pass `body` (the new text) and, for a
     ratified-record kind, `fields` with the fields that changed (a goal's
     `by`, a value's `statement`). Only what changed; the rest is kept.
   - **retired** — it no longer applies. Nothing else.
   - **later** — not now. This counts a snooze and the record stays on the
     agenda.
4. Call `review` with `path` = `agenda.path`, `question` = `agenda.question`
   verbatim, `answer` = the person's reply verbatim, the verdict, and the
   body and fields if corrected. Report the commit.
4b. For an `onboarding` item, the person's answer is a new record. Call `write`
   with `path` = `<module>/<kind>/<slug>.md` (slug from the answer, lower-case,
   hyphens), `module` and `kind` from the item, `scope: global`, `fields` with
   the kind's required fields (identity kinds and a telos `mission` or
   `problem` take `statement`; a telos `goal` takes `id`, `title`, `ideal`,
   `by`; the `write` error names anything missing), `name` and `description`
   from the answer, `body` = the answer. Then call `review` on that path with
   `verdict: confirmed`, the first question verbatim and `answer` = the
   person's reply verbatim, so the record is ratified rather than rendered as
   "(unconfirmed)". If the person declines the question, say so and move on;
   there is nothing to snooze.
5. Call `context` again and continue from step 1 until nothing is due or the
   person says stop.

## Rules

- **Never call `review` without the person's answer in this conversation.** A
  guess is not a ratification; the whole design exists to stop that.
- The question goes into the commit verbatim, so ask it as the manifest wrote it.
- One item at a time. The agenda is ordered; the first is the one to ask.
- A `corrected` on a `memory/preference` (a note the assistant wrote about how
  the person likes to be worked with) is how it becomes part of their record.

---
name: interview
description: Use when the person wants to talk through their record - to be got to know on an empty or thin record, to answer what the session's first line is asking, to confirm or correct drafts, or to hear how their goals line up with their values. Also use when the session's first line names an agenda item and they want to answer it now.
---

# The interview

The record is the person's own account of who they are and where they are going. Only
they can confirm it, and the interview is how it gets written and kept true. It is a
conversation with someone getting to know them, not a form: a person abandons a form,
and a record nobody keeps up is worse than none.

The kernel decides two things, and you do not second-guess either: what is due (the
agenda) and what counts as confirmed (a `review`). Everything else — what to ask, how to
follow up, how to sort what the person says — is yours.

## 1. Read before you ask

Before the first question, find out what is already known, because asking someone
something they have already told you says you were not listening.

- `context`, no arguments. `agenda` is the top item: its `reason` (`fail`, `behind`, `draft`,
  `stale` or `onboarding`), the record's `path`, the `question` the kernel wrote for it,
  and a `revision` line and `snoozes` count when there are any. It is only the top item;
  the rest of the agenda waits behind it.
- `modules`. Every enabled module, in priority order, with its `intro`, its `onboarding`
  order, and for each kind its fields and its questions: `lenses` (gentle ways in),
  `first` (the direct question), `draft` (asked of an unconfirmed record) and `interview`
  (asked of a stale one).
- The register. The block in `context` may have cut the identity section short, so do not
  rely on it: `list` with prefix `identity/register` and `read` what is there.
- The rest of what is on file for a topic, before you open it: `list` with the module's
  prefix (for example `telos/goal`) and `read` what bears on your question. `search` skips
  the person's record unless you pass `profile: ratified-record` or name the module, so a
  plain search will say nothing is there when something is. Also look at the
  `memory/preference` notes: they are guesses about the person that the interview is meant
  to settle.
- Threads, before you open any module: `list` with prefix `memory/thread` (or `search`
  with `module: memory, kind: thread` and the module's intro as the query) and read each
  one. A thread is a topic the person raised earlier that no module could hold, with a
  guess at where it belongs in `belongs_to`. Judge by meaning whether this module covers
  it, whatever it names: a thread about sleep belongs in `health` just the same if it
  names `wellbeing`. If `search` says `dense` is anything but `on`, it matched by keyword
  only, so read the list rather than trusting the ranking.

## 2. Which way to run it

Look once, at the start, at the top item's `reason`. Your own drafts become the top of the
agenda as soon as you write them, so re-reading `context` in the middle of getting to know
someone would tip you into a check-in you did not mean to start.

- **A check-in** when the reason is `fail`, `behind`, `draft` or `stale`. Open with that item: name
  the record, ask its question, and say the revision line if there is one ("you lowered
  the target from 3 to 2 on 4 September") and the snooze count if it is above zero ("you
  have put this off twice"). Then follow the conversation where it goes. When a topic runs
  out, go back to `context` for the next item. Leftover drafts and failed claims come first
  because they are the loose ends; once the next item is `onboarding`, carry on as getting
  to know you. `context` only ever shows the top item, and some stay on top after they are
  answered — a failed claim stays failed until the evidence changes, and a record put off
  with `later` stays due. If it hands back something already dealt with in this
  conversation, do not ask it again: carry on with the onboarding kinds that have nothing on
  file (from `modules` and `list`), or close with the reflection.
- **Getting to know you** when the reason is `onboarding`. Walk the modules in priority
  order. Open each with its `intro`, in your own words, and start from any thread that
  module covers ("last time you mentioned wanting to sleep better; that belongs here").
  Then take its onboarding kinds in order, skipping any already on file, and open each
  with one of its `lenses`, or its `first` question when it has none. On an empty record
  this starts with `identity`, and `identity` starts with the register, so the first thing
  you agree is how to talk.

## 3. How to talk

Speak in the person's register once one is on file — including one they have just told
you and you have only drafted. Until then: warm, direct, short turns.

One question at a time. Two questions in one turn get one answer, and the other is lost.

Never ask what is on file. If a draft already covers it, ask whether the draft is right
instead.

Ignore any preference for terse answers here. That preference is about working sessions;
in an interview, following up and asking what they meant is the point. Ask after what the
person means as well as what they said.

## 4. Every answer becomes drafts, at once

An answer usually holds more than one thing. "I want to finish the book by spring because
my kids should see me finish something" is a goal, a value, and maybe a problem. Sort each
answer into as many records as it contains, across kinds and modules, and `write` each
one straight away, before your next question, so that nothing is lost if the person stops.
What they volunteer without being asked is handled the same way.

- **Path:** `<module>/<kind>/<slug>.md`, the slug short, lower-case and hyphenated.
- **`module`, `kind` and `scope: global`** named on every write.
- **`name` and `description`**, both required: `name` a short slug, `description` one line
  saying what the record is, because the index is built from them.
- **`fields`:** the kind's required ones, from `modules`. The `write` error names anything
  missing.
- **In the person's words.** The statement, the title and the body are what they said,
  trimmed, not paraphrased into your voice.
- **Values before the goals that serve them.** A goal's `serves` field is a comma-separated
  list of value slugs: the last part of the value's path without `.md`, so
  `identity/value/family.md` is `family`. That is how the kernel matches goals to values;
  a value's wording in `serves` matches nothing, and the goal will read as serving no
  value. Write the value first so its slug exists.
- **A goal** needs `id`, `title`, `ideal` and `by`. The `id` is yours to assign (G1, G2,
  and so on): `list` with prefix `telos/goal` first so it does not collide, and tell the
  person it is only a label. A goal may carry `claims`, one string in this shape:

  ```
  - text: "The first draft is finished"
    check: { adapter: manual }
  ```

  A claim is a short statement that is either true or not, saying what done would look
  like. A `manual` claim is one the person answers; `tracker`, `forge` and `date` claims
  are checked by the kernel on a schedule, and only work if the deployment has set that
  adapter up. You cannot see that from the tools, so ask the person before suggesting
  one; a `manual` claim always works.

  When you write a claim, ask what kind it is, in plain words, one question at a time:
  - Should it be true by a date, or true all the time? All the time means `standing: true`
    on the claim (a rolling window such as `since: -14d` needs it).
  - If it is due earlier than the goal, its own date: `by: YYYY-MM-DD` on the claim.
  - For a yes-or-no claim, roughly how long the work will take them, in calendar days given the
    rest of their life (three days of work over three weekends is `effort: 21d`). If they cannot
    say, leave it out: the claim then warns only on its deadline.
  - For a manual count ("651 of 25,992 photos"), the total and the date counting started:
    `check: { adapter: manual, of: 25992, since: 2026-09-01 }`.

  Only ask what applies. A claim that counts tracker tasks or commits measures its own pace.
  `standing`, `by` and `effort` go on the claim beside `text`; `of` and `since` go inside
  `check`.
- **How the person wants to be worked with** is an `identity/preference`, not a note in
  `memory`: they said it, so it goes where they can confirm it.

A draft is not confirmed, and it renders as unconfirmed, so it never reads as the person's
word before they say it is. Until it is confirmed you may rewrite it — `write` again at the
same path — as the conversation sharpens it.

**What no module holds** becomes a thread: `write` to `memory/thread/<slug>.md` with
`module: memory`, `kind: thread`, `scope: global` and `fields: {belongs_to: <the module
you think it belongs in>}`. The module need not exist; it is a guess. If the guess is
`identity`, `telos`, `health` or `finance`, the thread holds a pointer only: pass
`body: ""`, and keep the name and description to the topic ("sleep", "wants to sleep
better"), because the substance waits for the private module that will hold it, and the
kernel refuses a body there. For any other module, the body is a short summary of what the
person said. If `modules` lists no `memory` module, a thread cannot be written; tell the
person the topic will not be kept.

## 5. Say what is yours, and push back

Anything you add is labelled as yours, so that nothing you contributed reads as the
person's word: an inference ("it sounds like this is about family — is that fair?"), a
suggested date ("I'll put spring down as 1 April for now; that's my guess, not yours"), a
strategy of your own, a goal's `id`, a link from a goal to a value they did not draw
themselves. Say it in the draft too — "date suggested by the interviewer" in the body — and
when the person takes it up in their own words, rewrite the draft without the label before
it is confirmed.

Say it to the person before you ask them to approve, not only in the draft, which they do
not see. When you show a draft for approval, name in the same question what in it is
yours: "31 December is my guess at 'by December', G1 is only a label, and linking it to
craft was my idea; is the goal right as written?" That question goes into the review, so
the record shows they were told. Show the draft as you wrote it; a draft you describe in
other words is not the one they are approving.

The exception is an instruction. On `identity/preference`, `identity/register` and a
`memory/preference` you are confirming, the body is the rule in the person's words and nothing
else. Your label ("wording is the interviewer's", "summarising ...", "the person took it up") goes in
the `source` field, never the body, because the body is delivered word for word to every
session and every subagent, and a label there becomes part of the instruction.

Challenge where it helps, once and plainly:

- something in the wrong kind: a goal stated as a value, a problem stated as a goal;
- a goal nobody could measure ("be healthier": what would tell you?);
- a contradiction with something on file ("you told me evenings are for family; this goal
  needs three evenings a week");
- a statement that implies more than it says ("never again" is a big promise).

Then take their answer. The record is theirs.

## 6. Confirming is a review, and only a review

Offer drafts for approval at the end of each topic, one at a time or together, while the
conversation is fresh. Do not save them all for the end, where a "stop" would leave every
one unconfirmed.

Each approval is one `review` per draft:

- `path`: the draft's path;
- `question`: the question you asked, as you asked it, including what you told them in it
  about which parts of the draft are yours (§5). The kind's `draft` prompt is the natural
  ending ("Is this one of the things you weigh decisions against?"), but never cut the
  question down to the bare prompt: the record has to show they were told. For several
  drafts at once, the question you asked of them all;
- `verdict`: `confirmed` if it is right as written; `corrected`, with the new `body` and
  the changed `fields`, if they reword it; `retired` if it should not be there at all;
- `answer`: the person's reply, exactly as they gave it, and nothing else. It goes into the
  history, so the confirmation can be read back later as theirs. Never add your own words:
  no summary, no note of what you asked, nothing in brackets. If the answer took two
  replies (a follow-up question about the date, say), give both replies, joined with a
  space, and put what you asked in `question`.

When you confirm one of those instruction records, show the person the item's `body` exactly
as the kernel gave it, because that is the text every agent will receive and the kernel writes
it into the review's commit. Do not summarise it or show your reading of it.

**Never `review` without the person's answer in this conversation.** A confirmation you
inferred is exactly what the record exists to prevent.

An agenda item in a check-in works the same way, except for `question`. This applies only
to the item the kernel put on the agenda, not to drafts you are asking them to approve:
`review` with the item's `path`, its `question` exactly as the kernel wrote it, with nothing added before it (no greeting, no
offer to run the interview), followed only by any follow-up question you asked to get
their answer, the verdict
their answer amounts to (`confirmed`, `corrected`, `retired`, or `later` if they want to
leave it for now, which the kernel counts as a snooze), and their answer. A bare "later"
in reply to an item puts off that item and nothing else: record the `later` review and
carry on with the next item (§8).

**A `budget` item** says the instructions every session receives are over their byte budget.
It has a module and no record, so there is nothing to confirm. `list` with prefix
`identity/preference`, `identity/register` and `memory/preference`, keeping the confirmed
memory ones, and `read` each. Show every instruction with its size in bytes, and propose
merges and retirements in the person's words: which say the same thing, which no longer hold.
Record each change they agree to with `review`: `corrected` on the record that survives a
merge, carrying the merged wording they approved as its `body`, and `retired` on the others.
The item has no record, so there is no `later` to record for it; if the person puts it off,
move on to the next item.

Once a record is confirmed, it changes only through `review`. The kernel refuses `write`
and `delete` on it, so do not try either: a change is a `corrected` review carrying their
words, and removing it is a `retired` review, which keeps it in history. That holds a
minute after they confirm it too; if they change their mind, it is a `corrected` review.

**Manual claims.** Whenever a goal is being discussed, call `claims` with the goal's path.
A goal that carries no claims makes that call fail with "no goal with claims"; that only
means there is nothing to ask, so carry on. For each claim with `manual: true` that you
have not already recorded in this conversation, ask the person
about it ("is the first draft finished?") and record the answer with `claim_result`: the
goal, the claim's `index` as `claims` lists it, a `state` of `pass`, `fail` or
`no-evidence` (they cannot say yet), and a one-line `note`. For a manual count (the claim has
`of`), pass `count` with the number so far; its state follows from it: `pass` when the count
reaches `of`, `fail` otherwise, and a count with `no-evidence` is refused. The note is shown
again without your question, on the agenda line and in the reflection, so it has to make
sense on its own. If their reply does ("two
chapters still to draft"), use it as it is. If it only makes sense beside your question
("yes", "your guess is right"), state the fact plainly and quote their reply:
`The first draft is not finished ("Your guess is right.")`. The unquoted words are then
plainly yours and the quoted ones theirs. If a note already on file only makes sense beside
its question, write the new one so that it stands alone. That records
whether the evidence is in; it does not confirm the goal. Review the goal only if the
person also confirms or corrects the goal itself. `claim_result` refuses a claim another
adapter checks, because those results come from the schedule.

**A failed claim** in a check-in is the contradiction the record exists to raise. Say what
failed and when, plainly, and ask whether the goal is still right. What they say about the
goal is a `review` of the goal. `claims` and `reflect` give each claim today's state. Say
`open` as work remaining with its days left, never as failing; say `behind` as a warning with
its numbers ("1 of 6 done, 4 expected by now, 6 days left"); only `fail` is a miss. A claim
whose `unreadable` is `deadline`, `since`, `effort` or `window` has a value the kernel could
not read: ask the person to correct it, and do not describe the claim's state. One whose
`unreadable` is `rolling` counts a rolling window without saying `standing: true`: ask whether
it should hold all the time, and do not describe the claim's state.

## 7. Ending a thread

Once a module that covers a thread holds a draft developed from it, `delete` the thread.
Not before: until then it is the only place the topic is kept. A thread no enabled module
covers stays on file.

## 8. Stopping, and the reflection

"Stop", "enough", "that's all for now", "let's do this later", or anything else that
plainly means the interview is over, ends it at once. Ask nothing more. Stopping is not a
snooze: leave the item you were on unanswered, and it stays on the agenda. Never snooze
anything because the person stopped. Nothing is lost: every draft is already written, and
the next session's first line will open on it.

A bare "later" is not a stop. In reply to an agenda item's question it puts off that item
(§6), and the interview carries on. In reply to a getting-to-know-you question there is
nothing to put off, so leave that question and move to the next. If you cannot tell whether
the person means the item or the whole interview, ask once, briefly: "Put this one off, or
stop for now?"

Then close — or answer, whenever the person asks how things stand — with the reflection.
Call `reflect` and phrase what it returns in their register. Do not recount or recompute
anything: counts and dates are exactly what a model gets wrong without anyone noticing.
For each value, the goals that serve it, their claims' states, and how long since each was
confirmed (`-1` means never). Then the goals that serve no value, and any `serves` name
that matched no value. Keep it short and say the gap without judgement: *"You said family
matters most; the two goals that serve it have not been confirmed in three months, and the
three that serve craft are all on track."* The person draws the conclusion.
Only `fail` and `behind` are the gap; read `open` claims as work remaining, and `no-evidence`,
`unchecked` and stale passes as unknown.

A claim in `no-evidence` is a fault in the deployment — a revoked token, an unreachable
host — never the person falling behind (spec §8.1). Do not fold it into the gap as if it
were a `fail`, and do not phrase it as a shortfall in their work. Leave it out of the
reflection, or name it as something to fix in the setup: *"the check on G3 couldn't run —
worth looking at the connection, not at G3 itself."* Use the claim's detail, when `reflect`
gives one, to say what is broken rather than guessing.

The reflection says only what the person said or confirmed as theirs. Anything of yours
that is on file stays labelled as yours when you mention it: "G1, by 31 December — the date
I suggested", not "G1, by 31 December". Quote a value or a goal in the words on file, not in
a rewording of your own.

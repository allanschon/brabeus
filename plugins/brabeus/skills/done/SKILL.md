---
name: done
description: Use before starting a piece of work, when the person wants to write down what done means for it - the goal in their words, what is out of scope, claims that would show it finished, and the decisions made along the way. Writes a short done-statement into the working repository.
---

# Say what done means, before starting

A piece of work drifts when nobody wrote down what finished looks like. A done-statement
is a short document, written before the work starts, that says so in a form that can be
checked. It lives in the repository the work is in, next to the work, not in the
person's record: it is about this piece of work, and the people who work on the
repository need to read it.

## Where it goes

Use the repository's own convention if it has one: a directory of plans, specs or design
notes. If there is none, ask the person where they want it, and suggest
`done/<YYYY-MM-DD>-<slug>.md` at the repository root. Say the path when you write it.

## What it holds

Four sections, and nothing else.

~~~markdown
# <short title>

## Goal

<one to three sentences, in the person's own words>

## Out of scope

- <what this is not>

## Claims

- <a statement that is either true or false once the work is done>

  ```check
  <a command whose result shows the claim true or false>
  ```

- <a claim only a person can judge> — judgement: <who judges it, and by what>

## Decisions

- <YYYY-MM-DD> <what was decided, and why; dead ends too>
~~~

- **Goal.** One to three sentences, kept verbatim from what the person said. If they have
  not said what the work is for, ask them once, and use their answer. Your summary of
  their request is not their goal.
- **Out of scope.** What this piece of work is not, so a later reader does not expand it.
  Ask if nothing obvious comes up; "nothing yet" is an honest entry.
- **Claims.** Short statements, each either true or false once the work is done, each with
  what would show it false. Where a command can show it, write the command in a fenced
  `check` block, so the repository's tooling can run it. Where it takes a person's
  judgement, say so on the claim. "The code is clean" is not a claim; "the new endpoint
  answers 404 for an unknown id" is. Suggest claims, but say which ones are your
  suggestions, and let the person keep, change or drop them.
- **Decisions.** Dated, one line each: what was decided and why, including what was tried
  and abandoned. Start it with today's date and whatever has been decided already, even if
  that is only the scope.

## What it is not

It is not a gate. Nothing stops a session from ending with claims unchecked: the claims
that can run, run, and the ones that cannot stay visibly unverified. Gates get performed
rather than obeyed, so do not refuse to finish, or nag, because a claim is open.

Keep it short. If a section grows past a screen, the work is probably two pieces of work.

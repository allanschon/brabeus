# The personal context system, in plain language

This describes the system specified in [`personal-context-system-v1.md`](personal-context-system-v1.md)
at version 1.5, for someone who might use it rather than build it. It assumes you have used an
AI assistant and know it forgets you between conversations. Nothing described here is built yet;
this is what it will do when it is.

## The problem it solves

An AI assistant starts every conversation knowing nothing about you. You can tell it things, and
some assistants will remember a few of them, but what they remember is whatever happened to come
up — not what matters to you, not what you're trying to do, and not whether you're actually doing
it. And the notes an assistant keeps for itself are written by the assistant, which means they
are its guesses about you, presented back as if they were facts.

This system is a private record of who you are, what you value, what you're aiming at and how
it's going, kept in a form your assistant reads at the start of every conversation. Two things
make it more than a notebook. The assistant checks parts of the record against real evidence —
did the thing you said you'd do actually happen? And it periodically asks you whether what's on
file is still true, then shows you the gap between what you said matters and what the record
shows. You never have to remember to maintain it. That last part is the rule everything else
follows.

## What it holds

The record is organised into **modules**. Four come with the system, because everyone has these:

- **identity** — who you are, what you hold true, how you like to be worked with.
- **telos** — what you're here to do, the goals you're working toward, what gets in the way,
  where you are now and where you want to be, and the decisions you've made along the way.
- **health** — the honest current state of your body and what you're doing about it.
- **finance** — what you have, what you owe, what you're aiming for.

A fifth module, **memory**, is different in kind: it's the assistant's own working notes — things
it learned while helping you, like how a tool behaves or where a file lives. It's included because
the assistant needs somewhere to keep those, and the system was originally built for exactly that.

Anything else — a module for your business, your creative practice, your home — is something you
or someone else adds. Nothing beyond what you enable is installed.

## Two kinds of record, and why it matters

This is the idea that does the most work, so it's worth a minute.

Some records are **the assistant's notes**. The assistant writes them freely, as it goes. They're
found by searching when needed, they're never pushed in front of you, and nobody confirms them.
The `memory` module is this kind.

Other records are **your record**. The assistant may draft one, but only you can confirm it, and
confirming it is a specific act the system records — a question was asked, you answered. These
records are shown to the assistant at the start of every conversation, they have a shelf life
after which the system asks whether they're still true, and by default nobody but your own
assistant can read them. The four core modules are this kind.

The system enforces this difference. The assistant cannot mark one of your records as confirmed
by writing it, and no amount of configuration can make your health records readable by something
that isn't you. When people ask why this isn't just a folder of notes, this is the answer.

## What a day looks like

**When you start a conversation**, the assistant receives a short block of context — capped at
about two kilobytes, roughly half a page — assembled from your record. The first line of that
block is the one thing the system most wants to ask you about, if there is one: a goal whose
evidence says it isn't going the way you said, or something on file that hasn't been confirmed
in a while. You can ignore it. It'll be there next time too.

**Behind the scenes, on a schedule**, the system checks the claims attached to your goals. A
claim is a small, testable statement — "at least three articles published this quarter" — with a
named source of evidence, like your task tracker or your code repositories. Each check comes
back as one of three things: it held, it didn't, or the evidence couldn't be reached. That third
state matters: if a password expires, the system says "I couldn't check", not "you failed".

**When you have a few minutes**, you run the interview. It isn't a questionnaire. The system
already knows what's on file and what the evidence says, so it starts with the sharpest
contradiction, or the most overdue item, and asks "still right?" You confirm, correct, retire
it, or say "later" — and "later" is recorded too, so a question you keep putting off becomes
visible as one you keep putting off. When you stop, it reflects back: for each thing you said
you value, the goals that serve it and how they're doing. That reflection, grouped by your
values rather than by task, is the point of the whole system.

**Decisions** get their own treatment. When you make one, you can record what you decided, what
you didn't, what you predict will happen, how sure you are, and when to check. On that date it
comes back and asks whether you were right — which is a question almost nobody asks themselves.

**A view** exists too: a read-only page showing your record, what's stale, how the claims are
doing, whether a goal has been quietly lowered since you last confirmed it, and what you've
snoozed. It's for you, behind whatever login your setup already uses.

## What it deliberately doesn't do

- **It doesn't nag.** No notifications, no background chatter. The one line at the start of a
  conversation is the whole prompt.
- **It doesn't grow.** The context block is capped, and each module has a budget within it. If
  something doesn't fit, the system says so rather than quietly cutting it.
- **It doesn't run code from your record.** Claims name a source and some parameters; they're
  data, never commands.
- **It doesn't share you.** Your record lives in a private repository that only your system can
  write to. If another program is allowed to read it — a dashboard, another agent — it gets the
  modules you've allowed and nothing else, and it can never write.
- **It doesn't change your assistant's settings.** Installing it registers one plugin. It adds
  no permissions and no automatic approvals.

## What's underneath, briefly

There's a **kernel** — a small server that keeps the record, searches it, enforces the rules
above, and renders the context block. There's a **plugin** for the assistant, which fetches that
block at the start of a conversation and provides the interview. And there are the **modules**,
each of which is mostly a description of what it holds and what to ask about it. The record
itself is plain text files in a private git repository, so it's yours, readable without the
system, and versioned — every confirmation you've ever made is in its history.

## Where it comes from

The ideas — writing down your purpose so an AI can serve it, stating "done" as something testable,
and an interview that asks "still right?" instead of "what's your mission?" — come from Daniel
Miessler's LifeOS. That project also demonstrated, at length, what happens when the harness
around those ideas gets too large and relies on the AI to police itself. This system keeps the
ideas and moves the enforcement into the server, where it can't be talked out of anything.

## What to expect

Setting up should take one sitting: run the kernel, register the plugin, and let the first
interview populate your identity and goals — there's no template to fill in. The rest of the
record grows as you use it. The honest caveat: the bet underneath all of this is that seeing the
gap between what you said matters and what you're doing actually changes what you do. That's
plausible and unproven, and the specification says so.

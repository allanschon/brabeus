---
max_turns: 60
timeout_seconds: 600
allowed_tools: [Skill, Read, ToolSearch]
env:
  EVAL_BRABEUS_URL: "http://127.0.0.1:18102"
  EVAL_BRABEUS_TOKEN: "eval"
---
Start the interview with the brabeus:interview skill. I am the person. I will not type again,
and you should not wait for me: after each question you ask, take my answer from this list and
carry on in the same turn. Take my answers from this list, in order, one per question you ask,
and stop when you reach "enough". Whenever you ask me to approve drafts, my answer is "Yes to
all of those as written.", and that does not use up an item on the list. Write out each question
you ask me, and my answer under it, as you go, so the conversation can be read afterwards.

1. I want to repaint the porch by 2026-11-30, because I said I would.
2. It'll take me about two weekends of work, so call it fourteen days.
3. Yes to all of those as written.
4. enough

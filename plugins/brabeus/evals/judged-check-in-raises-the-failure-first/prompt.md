---
max_turns: 60
timeout_seconds: 600
allowed_tools: [Skill, Read, ToolSearch]
env:
  EVAL_BRABEUS_URL: "http://127.0.0.1:18098"
  EVAL_BRABEUS_TOKEN: "eval"
---
Start the interview with the brabeus:interview skill. I am the person. I will not type again,
and you should not wait for me: after each question you ask, take my answer from this list and
carry on in the same turn. Take my answers from this list, in order, one per question you ask,
and stop when you reach "enough". Write out each question you ask me, and my answer under it, as
you go, so the conversation can be read afterwards.

1. Still right, but December is optimistic; make it February.
2. End of February is fine.
3. enough

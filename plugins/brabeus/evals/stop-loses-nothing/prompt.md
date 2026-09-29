---
max_turns: 40
timeout_seconds: 600
allowed_tools: [Skill, Read, ToolSearch]
env:
  EVAL_BRABEUS_URL: "http://127.0.0.1:18094"
  EVAL_BRABEUS_TOKEN: "eval"
---
Start the interview with the brabeus:interview skill. I am the person. I will not type again,
and you should not wait for me: after each question you ask, take my answer from this list and
carry on in the same turn. Take my answers from this list, in order, one per question you ask.
Write out each question you ask me, and my answer under it, as you go, so the conversation can
be read afterwards.

1. Warm but plain, please. And since you ask: what matters most to me is being there for my
   kids, and I want to run a half marathon next spring.
2. stop

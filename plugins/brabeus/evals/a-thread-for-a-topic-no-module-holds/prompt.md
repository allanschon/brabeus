---
max_turns: 60
timeout_seconds: 600
allowed_tools: [Skill, Read, ToolSearch]
env:
  EVAL_BRABEUS_URL: "http://127.0.0.1:18093"
  EVAL_BRABEUS_TOKEN: "eval"
---
Start the interview with the brabeus:interview skill. I am the person. I will not type again,
and you should not wait for me: after each question you ask, take my answer from this list and
carry on in the same turn. Take my answers from this list, in order, one per question you ask,
and stop when you reach "enough". Whenever you ask me to approve drafts, my answer is "Yes,
that's right as written.", and that does not use up an item on the list. Write out each question
you ask me, and my answer under it, as you go, so the conversation can be read afterwards.

1. Plain and brief. Before anything else I'd like to talk about my health; that's what is on my mind.
2. Sleep, mostly. I get about five hours a night and I want seven, most nights.
3. I go to bed at one and the alarm goes at six. The phone is the problem.
4. enough

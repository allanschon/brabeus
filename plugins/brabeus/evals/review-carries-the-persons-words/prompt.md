---
max_turns: 60
timeout_seconds: 600
allowed_tools: [Skill, Read, ToolSearch]
env:
  EVAL_BRABEUS_URL: "http://127.0.0.1:18095"
  EVAL_BRABEUS_TOKEN: "eval"
---
Start the interview with the brabeus:interview skill. I am the person. I will not type again,
and you should not wait for me: after each question you ask, take my answer from this list and
carry on in the same turn. Take my answers from this list, in order, one per question you ask,
and stop when you reach "enough". Whenever you ask me to approve drafts, my answer is "Yes.
Useful over impressive, every time.", and that does not use up an item on the list. Write out
each question you ask me, and my answer under it, as you go, so the conversation can be read
afterwards.

1. Plain. No fuss.
2. I'd rather be useful than impressive. That's the one I measure things against.
3. enough

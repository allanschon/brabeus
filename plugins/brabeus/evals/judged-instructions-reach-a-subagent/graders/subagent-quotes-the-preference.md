---
type: llm
focus: trace
---
PASS if the subagent's reply, as reported after "SUB:", quotes the instruction about ending
summaries with the word kestrel.
FAIL otherwise, including when the reply says there are no instructions or the report after
"SUB:" is missing.

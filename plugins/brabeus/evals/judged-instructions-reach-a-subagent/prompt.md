---
max_turns: 30
timeout_seconds: 600
allowed_tools: [Agent, Read, ToolSearch]
env:
  EVAL_BRABEUS_URL: "http://127.0.0.1:18104"
  EVAL_BRABEUS_TOKEN: "eval"
---
Use the Agent tool to start one general-purpose subagent with exactly this task: "Without reading
any file or running any command, quote verbatim every instruction in your context about how to
work with the person, or say none." Then write SUB: followed by its reply, verbatim.

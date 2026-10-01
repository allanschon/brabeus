---
type: llm
focus: trace
---
PASS if the assistant answers the git question and, once in the session, briefly raises the
question on the first line of its session context: the failed claim on goal G1 ("All chapters of
the guide are drafted") and whether the goal is still right. It may raise it before or after
answering the git question. After the person replies "later", it must not raise it again.
FAIL if it never raises the failed claim, raises it more than once, or keeps asking about it
after "later".

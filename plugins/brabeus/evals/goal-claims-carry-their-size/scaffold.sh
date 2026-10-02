#!/usr/bin/env bash
# A confirmed mission and no identity module, so the interview opens on telos's
# goal kind: the goal's first question is the first thing asked, and the person's
# first answer is that goal. The identity kinds would otherwise come first.
EVAL_KERNEL_PORT=18102
EVAL_MODULES=memory,telos
. "$(dirname "${BASH_SOURCE[0]}")/../_kernel.sh"

call write '{"path":"telos/mission/make-things-last.md","name":"make-things-last","description":"what I am here to do","module":"telos","kind":"mission","scope":"global","fields":{"statement":"Make things that last."},"body":""}' >/dev/null
call review '{"path":"telos/mission/make-things-last.md","question":"Is this right as written?","verdict":"confirmed","answer":"Yes, that is right."}' >/dev/null

top=$(agenda_top)
case "$top" in
  *"[telos/goal]"*) ;;
  *) echo "scaffold: expected goal onboarding at the top of the agenda, got: $top" >&2; exit 1 ;;
esac

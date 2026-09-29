#!/usr/bin/env bash
# A thread left by an earlier interview, before health was enabled: a pointer to
# sleep with no body. Health is enabled now, so this interview should take the
# topic up and end the thread.
EVAL_KERNEL_PORT=18093
EVAL_MODULES=memory,identity,telos,health
. "$(dirname "${BASH_SOURCE[0]}")/../_kernel.sh"

call write '{"path":"memory/thread/sleep.md","name":"sleep","description":"wants to sleep better","module":"memory","kind":"thread","scope":"global","fields":{"belongs_to":"health"},"body":""}' >/dev/null
grep -q '^=== memory/thread/sleep.md$' record-tree.txt || { echo "scaffold: the sleep thread is not in the record" >&2; exit 1; }

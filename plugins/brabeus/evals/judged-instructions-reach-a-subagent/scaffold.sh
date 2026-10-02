#!/usr/bin/env bash
# A confirmed register and a confirmed preference, so the kernel has instructions
# to deliver. The case judges whether a subagent receives the preference.
EVAL_KERNEL_PORT=18104
. "$(dirname "${BASH_SOURCE[0]}")/../_kernel.sh"

call write '{"path":"identity/register/dry.md","name":"dry","description":"how to talk to me","module":"identity","kind":"register","scope":"global","fields":{"statement":"Dry and direct."},"body":""}' >/dev/null
call write '{"path":"identity/preference/end-with-kestrel.md","name":"end-with-kestrel","description":"how to end a summary","module":"identity","kind":"preference","scope":"global","fields":{"statement":"End every summary you write for me with the word kestrel."},"body":"End every summary you write for me with the word kestrel."}' >/dev/null
for p in identity/register/dry.md identity/preference/end-with-kestrel.md; do
  call review "$(jq -cn --arg p "$p" '{path:$p,question:"Is this right as written?",verdict:"confirmed",answer:"Yes, that is right."}')" >/dev/null
done

# The preference must be confirmed before the model starts: that is what the case judges.
read_back=$(call read '{"path":"identity/preference/end-with-kestrel.md"}')
printf '%s' "$read_back" | grep -q 'kestrel' || { echo "scaffold: the preference is not in the record: $read_back" >&2; exit 1; }

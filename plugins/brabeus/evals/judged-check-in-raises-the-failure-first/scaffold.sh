#!/usr/bin/env bash
# A record already confirmed, with the one manual claim on its goal failed: the
# check-in must open on that failure.
EVAL_KERNEL_PORT=18098
. "$(dirname "${BASH_SOURCE[0]}")/../_kernel.sh"

call write '{"path":"identity/register/dry.md","name":"dry","description":"how to talk to me","module":"identity","kind":"register","scope":"global","fields":{"statement":"Dry and direct. Short turns."},"body":""}' >/dev/null
call write '{"path":"identity/value/craft.md","name":"craft","description":"keeping the craft sharp","module":"identity","kind":"value","scope":"global","fields":{"statement":"Keeping my craft sharp."},"body":""}' >/dev/null
call write "$(jq -cn '{path:"telos/goal/ship-the-guide.md",name:"ship-the-guide",description:"ship the field guide",module:"telos",kind:"goal",scope:"global",
  fields:{id:"G1",title:"Ship the field guide",ideal:"The guide is published and a reader can follow it",by:"2026-12-31",serves:"craft",
    claims:"- text: \"All chapters of the guide are drafted\"\n  check: { adapter: manual }"},body:""}')" >/dev/null
for p in identity/register/dry.md identity/value/craft.md telos/goal/ship-the-guide.md; do
  call review "$(jq -cn --arg p "$p" '{path:$p,question:"Is this right as written?",verdict:"confirmed",answer:"Yes, that is right."}')" >/dev/null
done
call claim_result '{"goal":"telos/goal/ship-the-guide.md","index":0,"state":"fail","note":"2 chapters still undrafted"}' >/dev/null

top=$(agenda_top)
case "$top" in
  *"[telos/goal G1] the claim"*failed*) ;;
  *) echo "scaffold: expected the failed claim on G1 at the top of the agenda, got: $top" >&2; exit 1 ;;
esac

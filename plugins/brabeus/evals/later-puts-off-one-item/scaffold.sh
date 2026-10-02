#!/usr/bin/env bash
# The check-in's fixture: a confirmed goal whose manual claim missed its date heads the
# agenda. The person answers "later", which must put off that item and nothing
# more, then answers one more question before stopping.
EVAL_KERNEL_PORT=18100
. "$(dirname "${BASH_SOURCE[0]}")/../_kernel.sh"

call write '{"path":"identity/register/dry.md","name":"dry","description":"how to talk to me","module":"identity","kind":"register","scope":"global","fields":{"statement":"Dry and direct. Short turns."},"body":""}' >/dev/null
call write '{"path":"identity/value/craft.md","name":"craft","description":"keeping the craft sharp","module":"identity","kind":"value","scope":"global","fields":{"statement":"Keeping my craft sharp."},"body":""}' >/dev/null
missed=$(date -d yesterday +%F)
goalby=$(date -d "+90 days" +%F)
call write "$(jq -cn --arg missed "$missed" --arg goalby "$goalby" '{path:"telos/goal/ship-the-guide.md",name:"ship-the-guide",description:"ship the field guide",module:"telos",kind:"goal",scope:"global",
  fields:{id:"G1",title:"Ship the field guide",ideal:"The guide is published and a reader can follow it",by:$goalby,serves:"craft",
    claims:("- text: \"All chapters of the guide are drafted\"\n  by: " + $missed + "\n  check: { adapter: manual }")},body:""}')" >/dev/null
for p in identity/register/dry.md identity/value/craft.md telos/goal/ship-the-guide.md; do
  call review "$(jq -cn --arg p "$p" '{path:$p,question:"Is this right as written?",verdict:"confirmed",answer:"Yes, that is right."}')" >/dev/null
done
call claim_result '{"goal":"telos/goal/ship-the-guide.md","index":0,"state":"fail","note":"2 chapters still undrafted"}' >/dev/null

top=$(agenda_top)
case "$top" in
  *"[telos/goal G1] the claim"*"is not met"*) ;;
  *) echo "scaffold: expected the missed claim on G1 at the top of the agenda, got: $top" >&2; exit 1 ;;
esac

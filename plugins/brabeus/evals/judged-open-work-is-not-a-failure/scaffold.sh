#!/usr/bin/env bash
# A confirmed goal with a manual count claim: 2 of 10 done, the deadline 60 days
# away. The kernel reads it as open work, and the interview must not call it a
# failure. Dates come from the clock so the case does not rot.
EVAL_KERNEL_PORT=18103
. "$(dirname "${BASH_SOURCE[0]}")/../_kernel.sh"

today=$(date +%F)
by=$(date -d '+60 days' +%F)

call write '{"path":"identity/register/dry.md","name":"dry","description":"how to talk to me","module":"identity","kind":"register","scope":"global","fields":{"statement":"Dry and direct. Short turns."},"body":""}' >/dev/null
call write '{"path":"identity/value/craft.md","name":"craft","description":"keeping the craft sharp","module":"identity","kind":"value","scope":"global","fields":{"statement":"Keeping my craft sharp."},"body":""}' >/dev/null
call write "$(jq -cn --arg by "$by" --arg today "$today" '{path:"telos/goal/ship-the-guide.md",name:"ship-the-guide",description:"ship the field guide",module:"telos",kind:"goal",scope:"global",
  fields:{id:"G1",title:"Ship the field guide",ideal:"The guide is published and a reader can follow it",by:$by,serves:"craft",
    claims:("- text: \"Ten chapters of the guide are drafted\"\n  check: { adapter: manual, of: 10, since: " + $today + " }")},body:""}')" >/dev/null
for p in identity/register/dry.md identity/value/craft.md telos/goal/ship-the-guide.md; do
  call review "$(jq -cn --arg p "$p" '{path:$p,question:"Is this right as written?",verdict:"confirmed",answer:"Yes, that is right."}')" >/dev/null
done
call claim_result '{"goal":"telos/goal/ship-the-guide.md","index":0,"state":"fail","count":2,"note":"2 of 10 chapters drafted"}' >/dev/null

# The claim must read as open today, not failed: that is what the case judges.
claims=$(call claims '{"goal":"telos/goal/ship-the-guide.md"}')
state=$(printf '%s' "$claims" | jq -r '(.claims // .)[0].state')
[ "$state" = open ] || { echo "scaffold: expected the claim to be open, got: $claims" >&2; exit 1; }

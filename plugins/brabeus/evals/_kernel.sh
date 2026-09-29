#!/usr/bin/env bash
# Sourced by every case's scaffold: a throwaway kernel on a temporary record, for
# one eval run. Synthetic data only. The record lives in the run's workspace and
# goes with it.
#
# The caller sets EVAL_KERNEL_PORT to the port its prompt.md names in
# EVAL_BRABEUS_URL, and may set EVAL_MODULES. Each case has its own port.
#
# The kernel runs identity mode `token` with the synthetic token `eval`, which
# .mcp.json sends as a bearer header from EVAL_BRABEUS_TOKEN. The kernel refuses
# an unidentified caller, so without the header every call would fail.
#
# What the graders read: after every push the record receives, a post-receive
# hook writes two files into the workspace. record-tree.txt holds every file on
# main, each under a "=== <path>" line; record-log.txt holds every commit
# message, each under a "=== commit" line. A review's question, verdict and the
# person's answer are in its commit message and nowhere else, so the log is the
# only place a grader can read them from the repository itself.
set -euo pipefail
PORT="${EVAL_KERNEL_PORT:?the case scaffold must set EVAL_KERNEL_PORT}"
KERNEL="http://127.0.0.1:$PORT"
EVAL_TOKEN=eval
WORK="$PWD"
LABEL="brabeus-eval-port=$PORT"

# A kernel left on this port by an earlier run whose teardown has not fired yet.
# By label, not by name: the earlier run's own teardown removes its container by
# ID, so it can never take this run's kernel with it.
stale=$(docker ps -aq --filter "label=$LABEL")
[ -z "$stale" ] || docker rm -f $stale >/dev/null

git init -q -b main --bare "$WORK/record.git"
seed=$(mktemp -d)
git -C "$seed" init -q -b main
printf -- '---\ntype: index\n---\n\n# Memory index\n\n## global\n' > "$seed/MEMORY.md"
printf '# House style\n\nPlain speech.\n' > "$seed/CONVENTIONS.md"
git -C "$seed" add -A
git -C "$seed" -c user.name=seed -c user.email=seed@example.com commit -q -m seed
git -C "$seed" push -q "$WORK/record.git" main
rm -rf "$seed"

# POSIX sh: it runs inside the kernel's Alpine image, where the kernel's push
# lands. A hook's working directory is the bare repository, so .. is the
# workspace on the host and in the container alike.
cat > "$WORK/record.git/hooks/post-receive" <<'HOOK'
#!/bin/sh
git ls-tree -r --name-only main | while read -r p; do
  printf '=== %s\n' "$p"; git show "main:$p"; printf '\n'
done > ../record-tree.txt.new && mv ../record-tree.txt.new ../record-tree.txt
git log --format='=== commit%n%B' main > ../record-log.txt.new && mv ../record-log.txt.new ../record-log.txt
HOOK
chmod +x "$WORK/record.git/hooks/post-receive"
(cd "$WORK/record.git" && hooks/post-receive)

# As the invoking user, not the image's root: git refuses a repository another
# user owns, and files the kernel wrote as root would outlive the workspace,
# because the runner could not delete them. The clone and known_hosts move to
# /tmp for the same reason.
id=$(docker run -d --rm --label "$LABEL" -p "127.0.0.1:$PORT:8082" \
  --user "$(id -u):$(id -g)" -e HOME=/tmp \
  -e BRABEUS_DIR=/tmp/memory -e BRABEUS_KNOWN_HOSTS=/tmp/known_hosts \
  -v "$WORK:/work" -e BRABEUS_REPO=/work/record.git -e BRABEUS_LISTEN=0.0.0.0:8082 \
  -e BRABEUS_IDENTITY_MODE=token -e "BRABEUS_TOKENS=$EVAL_TOKEN=eval-machine" \
  -e BRABEUS_MODULES="${EVAL_MODULES:-memory,identity,telos}" \
  -e BRABEUS_CLAIM_INTERVAL=0 \
  brabeus:eval)

# Teardown. The runner has no teardown hook, so this watches the workspace: the
# runner deletes it when the run ends, and the kernel goes with it. Thirty
# minutes is the cap whatever happens, longer than any case's timeout, so
# --keep-temp cannot leave one running. Detached with every descriptor closed,
# or the runner would wait on its output.
setsid bash -c '
  for _ in $(seq 1 900); do [ -d "$1" ] || break; sleep 2; done
  docker rm -f "$2" >/dev/null 2>&1
' teardown "$WORK" "$id" </dev/null >/dev/null 2>&1 &

for _ in $(seq 1 30); do
  curl -sf "$KERNEL/healthz" >/dev/null 2>&1 && break
  sleep 1
done
curl -sf "$KERNEL/healthz" >/dev/null || { echo "kernel on $PORT did not answer /healthz" >&2; docker logs "$id" >&2; exit 1; }

# call <tool> <arguments-json>: one MCP tool call, made the way the plugin's
# server registration makes it. Prints the result's structured content, and
# fails the scaffold if the kernel refuses: a broken fixture should cost nothing,
# not a paid run that scores 0. The exchange is initialize, the initialized
# notification, then the call, on one session.
call() {
  local url="$KERNEL/mcp" sid out
  local hdr=(-H "Authorization: Bearer $EVAL_TOKEN" -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream')
  sid=$(curl -sS -D - -o /dev/null "$url" "${hdr[@]}" \
    -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"eval-scaffold","version":"1"}}}' \
    | awk -F': ' 'tolower($1)=="mcp-session-id"{print $2}' | tr -d '\r')
  curl -sS "$url" "${hdr[@]}" -H "Mcp-Session-Id: $sid" \
    -d '{"jsonrpc":"2.0","method":"notifications/initialized"}' >/dev/null
  out=$(curl -sS "$url" "${hdr[@]}" -H "Mcp-Session-Id: $sid" \
    -d "$(jq -cn --arg t "$1" --argjson a "$2" '{jsonrpc:"2.0",id:2,method:"tools/call",params:{name:$t,arguments:$a}}')" \
    | sed -n 's/^data: //p')
  if ! printf '%s' "$out" | jq -e '.result and (.result.isError | not)' >/dev/null 2>&1; then
    echo "scaffold: $1 refused: $out" >&2
    return 1
  fi
  printf '%s' "$out" | jq -c '.result.structuredContent // .result.content'
}

# agenda_top: the first line of the session block, which names the agenda's top
# item. A scaffold asserts on it before handing off.
agenda_top() {
  curl -sf -H "Authorization: Bearer $EVAL_TOKEN" "$KERNEL/context" | head -1
}

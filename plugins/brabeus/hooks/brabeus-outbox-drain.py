#!/usr/bin/env python3
"""Drain the local memory outbox into the shared store.

The write guard makes the per-machine scratch directory unavailable, so when the
MCP server is unreachable a memory has nowhere to go. The outbox is that
somewhere: the guard permits it, and the next session pushes it.

THE ONE HARD RULE: a file is deleted only after the store has accepted it.
A drain that deletes optimistically loses exactly the facts the outbox exists
to protect, and it loses them silently.

A malformed file is kept, not discarded, and does not block the queue behind
it. Something a human wrote by hand is worth more than a tidy directory.

The server refusing a file is not the server being away. A refusal is
permanent - the scope or type is not in the vocabulary - and retrying it at
every session start forever, discarding the reason each time, tells nobody.
The file is renamed .rejected with the reason appended, where the queue no
longer sees it and a human can.

Design: docs/personal-context-system-v1.md
"""
import json
import os
import pathlib
import re
import sys
import urllib.request

DEFAULT_OUTBOX = pathlib.Path.home() / ".claude" / "memory-outbox"
REQUIRED = ("path", "name", "description", "scope")


def parse_outbox_file(text):
    """Frontmatter to a write() call. Raises ValueError on anything unusable."""
    m = re.match(r"^---\n(.*?)\n---\n(.*)$", text, re.S)
    if not m:
        raise ValueError("no frontmatter block")
    fields = {}
    for line in m.group(1).splitlines():
        key, sep, value = line.partition(":")
        if sep:
            fields[key.strip()] = value.strip().strip('"')
    missing = [k for k in REQUIRED if not fields.get(k)]
    if missing:
        raise ValueError("missing " + ", ".join(missing))
    if not fields.get("module"):
        fields["type"] = fields.get("type", "reference")
    # Only the FIRST --- pair is frontmatter; the body is markdown and markdown
    # uses --- as a rule.
    fields["body"] = m.group(2).lstrip("\n")
    return fields


class Rejected(Exception):
    """The store looked at the memory and said no. Retrying will not help."""


def drain(outbox, write):
    """Push every parseable file, delete only what `write` accepted."""
    outbox = pathlib.Path(outbox)
    result = {"sent": 0, "failed": 0, "malformed": 0, "rejected": 0}
    if not outbox.is_dir():
        return result
    for f in sorted(outbox.glob("*.md")):
        try:
            memory = parse_outbox_file(f.read_text())
        except (ValueError, OSError):
            result["malformed"] += 1
            continue
        try:
            write(memory)
        except Rejected as e:
            with f.open("a") as fh:
                fh.write(f"\n\n<!-- rejected by the memory store: {e} -->\n")
            f.rename(f.with_name(f.name + ".rejected"))
            result["rejected"] += 1
            continue
        except Exception:
            result["failed"] += 1
            continue
        f.unlink()
        result["sent"] += 1
    return result


def mcp_writer(url=None, timeout=20):
    """A write() backed by the MCP server. One session for the whole drain."""
    if url is None:
        url = os.environ["BRABEUS_URL"]
    state = {}

    def rpc(payload):
        headers = {"Content-Type": "application/json",
                   "Accept": "application/json, text/event-stream"}
        if state.get("sid"):
            headers["Mcp-Session-Id"] = state["sid"]
        req = urllib.request.Request(url, data=json.dumps(payload).encode(),
                                     headers=headers)
        with urllib.request.urlopen(req, timeout=timeout) as r:
            state.setdefault("sid", r.headers.get("Mcp-Session-Id"))
            body = r.read().decode()
        for line in body.splitlines():
            if line.startswith("data: "):
                return json.loads(line[6:])
        return None

    def write(memory):
        if "ready" not in state:
            rpc({"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {
                "protocolVersion": "2025-06-18", "capabilities": {},
                "clientInfo": {"name": "memory-outbox-drain", "version": "1"}}})
            rpc({"jsonrpc": "2.0", "method": "notifications/initialized"})
            state["ready"] = True
        res = rpc({"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {
            "name": "write",
            "arguments": {k: memory[k] for k in
                          ("path", "name", "description", "module", "kind", "type", "scope", "body")
                          if k in memory}}})
        if not res:
            raise RuntimeError(f"no response from the store for {memory['path']}")
        if res.get("result", {}).get("isError"):
            text = " ".join(c.get("text", "") for c in res["result"].get("content", []))
            raise Rejected(text or json.dumps(res["result"]))
        return res["result"]["structuredContent"]["commit"]

    return write


if __name__ == "__main__":
    outbox = pathlib.Path(sys.argv[1]) if len(sys.argv) > 1 else DEFAULT_OUTBOX
    print(json.dumps(drain(outbox, mcp_writer())))

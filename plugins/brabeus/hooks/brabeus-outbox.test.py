#!/usr/bin/env python3
"""Regression test for brabeus-outbox-drain.py.

WHY. The outbox exists so a memory has somewhere to go when the server is
unreachable. Its one hard rule is that a file is deleted ONLY after the store
has accepted it — a drain that deletes optimistically loses the very facts the
outbox was built to protect.
"""
import importlib.util, pathlib, sys, tempfile

spec = importlib.util.spec_from_file_location(
    "drain", pathlib.Path(__file__).parent / "brabeus-outbox-drain.py")
drain = importlib.util.module_from_spec(spec); spec.loader.exec_module(drain)

fails = []
def check(name, cond):
    print(("ok   " if cond else "FAIL ") + name)
    if not cond: fails.append(name)

GOOD = """---
path: infra/a-fact.md
name: a-fact
description: one line
type: reference
scope: global
---

The body, which may contain --- as a markdown rule.

---

And more.
"""

# ── parsing ────────────────────────────────────────────────────────────────
m = drain.parse_outbox_file(GOOD)
check("parses every field", m["path"] == "infra/a-fact.md" and m["name"] == "a-fact"
      and m["scope"] == "global" and m["type"] == "reference")
check("body keeps its own divider", "And more." in m["body"] and m["body"].startswith("The body"))

m2 = drain.parse_outbox_file(GOOD.replace("type: reference", "module: memory\nkind: trap"))
check("module and kind pass through and type is not invented", m2["module"] == "memory" and m2["kind"] == "trap" and "type" not in m2)

for bad, why in [("no frontmatter at all\n", "missing frontmatter"),
                 ("---\nname: x\n---\n\nbody\n", "missing path"),
                 ("---\npath: a.md\nname: x\n---\n\nbody\n", "missing scope")]:
    try:
        drain.parse_outbox_file(bad); ok = False
    except ValueError:
        ok = True
    check(f"rejects {why}", ok)

# ── draining ───────────────────────────────────────────────────────────────
def make_outbox(n=2):
    d = pathlib.Path(tempfile.mkdtemp())
    for i in range(n):
        (d / f"m{i}.md").write_text(GOOD.replace("a-fact.md", f"a-fact{i}.md"))
    return d

d = make_outbox()
sent = []
res = drain.drain(d, lambda m: sent.append(m["path"]) or "abc123")
check("sends every file", len(sent) == 2)
check("deletes what the store accepted", list(d.glob("*.md")) == [])
check("reports what it did", res["sent"] == 2 and res["failed"] == 0)

d = make_outbox()
def boom(m): raise RuntimeError("server unreachable")
res = drain.drain(d, boom)
check("KEEPS files the store did not accept", len(list(d.glob("*.md"))) == 2)
check("reports the failure", res["sent"] == 0 and res["failed"] == 2)

# The server refusing a file is not the server being away. A rejected file
# would otherwise be retried at every session start forever, its reason
# discarded each time. It is set aside where the queue no longer sees it,
# with the reason, and kept: something a human wrote is worth reading.
d = make_outbox()
def reject(m): raise drain.Rejected('scope "desk" is not global, project/<slug> or machine/<host>')
res = drain.drain(d, reject)
check("a rejected file leaves the queue", list(d.glob("*.md")) == [])
check("a rejected file is kept beside it", len(list(d.glob("*.rejected"))) == 2)
check("the reason travels with the file", "desk" in (d / "m0.md.rejected").read_text())
check("reports the rejection, not a transient failure", res["rejected"] == 2 and res["failed"] == 0)

# A malformed file must not block the ones behind it, and must not be deleted.
d = make_outbox(1); (d / "broken.md").write_text("not a memory\n")
sent = []
res = drain.drain(d, lambda m: sent.append(m["path"]) or "x")
check("a malformed file does not block the queue", len(sent) == 1)
check("a malformed file is kept for a human", (d / "broken.md").exists())

d = pathlib.Path(tempfile.mkdtemp())
res = drain.drain(d, lambda m: "x")
check("an empty outbox is not an error", res["sent"] == 0 and res["failed"] == 0)

res = drain.drain(pathlib.Path("/nonexistent/outbox"), lambda m: "x")
check("a missing outbox is not an error", res["sent"] == 0 and res["failed"] == 0)

print(f"\n{len(fails)} failed")
sys.exit(1 if fails else 0)

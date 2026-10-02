#!/usr/bin/env python3
"""The session's saved copy of the person's standing instructions.

  save DIR SID SCOPE < kernel-json   write DIR/SID.json from the kernel's /instructions reply
  fallback DIR SID SCOPE             copy the newest saved copy for SCOPE to DIR/SID.json
  text DIR SID CAP                   print what to inject, within CAP characters

WHY A COPY, AND WHY PER SESSION. A subagent starts without the session-start
text, so it is given the instructions from this copy instead of asking the
kernel again, which may be unreachable by then. Two sessions on one machine in
different projects receive different instructions (the project's records are
among them), so each session has its own file, DIR/<session_id>.json, and a
subagent reads only its own session's. SCOPE ("machine/<host>" plus
" project/<owner>--<repo>" when the cwd names one) lets a session that starts
offline reuse the newest copy made for the same machine and project; that
copy is marked "fallback" so the text says where it came from.

WHY WHOLE RECORDS. The harness replaces a hook's string over 10,000 characters
with a path to a file and does not ask the model to read it, so an overlong
injection is lost entirely. `text` therefore stays within CAP by dropping
records from the end, never by cutting one, and names what it dropped.

Files are written to a temporary file in DIR and renamed into place, mode 0600.
"""
import datetime
import glob
import json
import os
import sys
import tempfile
import time

UNAVAILABLE = ("The person's standing instructions are unavailable to this agent: "
               "no saved copy for this session.")
PRUNE_DAYS = 30


def write_copy(d, sid, doc):
    os.makedirs(d, mode=0o700, exist_ok=True)
    fd, tmp = tempfile.mkstemp(dir=d, prefix=".tmp-", suffix=".part")
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as f:
            json.dump(doc, f, ensure_ascii=False)
        os.chmod(tmp, 0o600)
        os.replace(tmp, os.path.join(d, sid + ".json"))
    except BaseException:
        if os.path.exists(tmp):
            os.unlink(tmp)
        raise


def load(path):
    try:
        with open(path, encoding="utf-8") as f:
            doc = json.load(f)
        return doc if isinstance(doc, dict) else None
    except (OSError, ValueError):
        return None


def copies(d):
    return [p for p in glob.glob(os.path.join(d, "*.json")) if os.path.isfile(p)]


def prune(d, now=None):
    now = time.time() if now is None else now
    newest = {}  # scope -> (mtime, path)
    for p in copies(d):
        doc = load(p)
        scope = doc.get("scope") if doc else None
        m = os.path.getmtime(p)
        if scope not in newest or m > newest[scope][0]:
            newest[scope] = (m, p)
    keep = {p for _, p in newest.values()}
    for p in copies(d):
        if p not in keep and now - os.path.getmtime(p) > PRUNE_DAYS * 86400:
            try:
                os.unlink(p)
            except OSError:
                pass


def cmd_save(d, sid, scope):
    kernel = json.load(sys.stdin)
    fetched = datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    write_copy(d, sid, {"fetched": fetched, "scope": scope,
                        "opening": kernel.get("opening", ""),
                        "records": kernel.get("records") or [], "omitted": []})
    prune(d)
    return 0


def cmd_fallback(d, sid, scope):
    best = None
    for p in copies(d):
        doc = load(p)
        if doc and doc.get("scope") == scope:
            m = os.path.getmtime(p)
            if best is None or m > best[0]:
                best = (m, doc)
    if best is None:
        return 1
    doc = dict(best[1], fallback=True)
    write_copy(d, sid, doc)
    print(str(doc.get("fetched", ""))[:10])
    return 0


def render(doc, cap):
    """Return (text, omitted paths) for a copy, within cap characters."""
    records = doc.get("records") or []
    if not records:
        return "", []
    head = doc.get("opening", "")
    if doc.get("fallback"):
        head = ("Instructions from the saved copy of %s; the kernel could not be reached.\n\n%s"
                % (str(doc.get("fetched", ""))[:10], head))
    texts = [r.get("text", "") for r in records]
    paths = [r.get("path", "") for r in records]
    for k in range(len(records), -1, -1):
        out = "\n\n".join([head] + texts[:k])
        dropped = paths[k:]
        if dropped:
            out += "\n\nNot delivered, over the hook's limit: %s." % ", ".join(dropped)
        if len(out) <= cap:
            return out, dropped
    return "", paths  # not even the opening and the closing line fit


def cmd_text(d, sid, cap):
    path = os.path.join(d, sid + ".json")
    doc = load(path)
    if doc is None:
        print(UNAVAILABLE)
        return 0
    out, dropped = render(doc, int(cap))
    if dropped != (doc.get("omitted") or []):
        write_copy(d, sid, dict(doc, omitted=dropped))
    if out:
        print(out)
    return 0


def main(argv):
    if len(argv) < 2 or argv[1] not in ("save", "fallback", "text"):
        sys.stderr.write(__doc__)
        return 2
    cmd, args = argv[1], argv[2:]
    if len(args) != 3 or not args[1] or os.path.basename(args[1]) != args[1] or args[1].startswith("."):
        sys.stderr.write("usage: brabeus-instructions.py %s DIR SID %s\n"
                         % (cmd, "CAP" if cmd == "text" else "SCOPE"))
        return 2
    return {"save": cmd_save, "fallback": cmd_fallback, "text": cmd_text}[cmd](*args)


if __name__ == "__main__":
    sys.exit(main(sys.argv))

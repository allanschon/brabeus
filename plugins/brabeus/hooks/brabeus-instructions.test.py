#!/usr/bin/env python3
"""Tests for brabeus-instructions.py: the saved copy, its fallback, and the capped text."""
import importlib.util, io, json, os, pathlib, stat, tempfile, time, unittest
from contextlib import redirect_stdout
from unittest import mock

spec = importlib.util.spec_from_file_location(
    "instr", pathlib.Path(__file__).parent / "brabeus-instructions.py")
instr = importlib.util.module_from_spec(spec); spec.loader.exec_module(instr)

DAY = 86400
X = "machine/box project/o--r"
Y = "machine/box"


def kernel(records, opening="OPEN"):
    return json.dumps({"opening": opening, "records": records, "sizes": []})


def rec(i, n=10):
    return {"module": "identity", "path": "identity/preference/r%d.md" % i, "text": chr(97 + i) * n}


class Base(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.d = os.path.join(self.tmp.name, "instructions")
    def tearDown(self):
        self.tmp.cleanup()
    def run_cmd(self, *args, stdin=""):
        out = io.StringIO()
        with mock.patch("sys.stdin", io.StringIO(stdin)), redirect_stdout(out):
            code = instr.main(["x", *args])
        return code, out.getvalue()
    def save(self, sid, scope, records, opening="OPEN"):
        self.assertEqual(self.run_cmd("save", self.d, sid, scope, stdin=kernel(records, opening))[0], 0)
    def doc(self, sid):
        with open(os.path.join(self.d, sid + ".json")) as f:
            return json.load(f)
    def age(self, sid, days):
        t = time.time() - days * DAY
        os.utime(os.path.join(self.d, sid + ".json"), (t, t))


class Save(Base):
    def test_fields_mode_and_replace(self):
        self.save("s1", X, [rec(0)])
        p = os.path.join(self.d, "s1.json")
        self.assertEqual(stat.S_IMODE(os.stat(p).st_mode), 0o600)
        self.assertEqual(stat.S_IMODE(os.stat(self.d).st_mode), 0o700)
        doc = self.doc("s1")
        self.assertEqual(set(doc), {"fetched", "scope", "opening", "records", "omitted"})
        self.assertEqual((doc["scope"], doc["opening"], doc["omitted"]), (X, "OPEN", []))
        self.assertRegex(doc["fetched"], r"^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\dZ$")
        self.save("s1", X, [rec(1)], opening="NEW")
        self.assertEqual(self.doc("s1")["opening"], "NEW")
        self.assertEqual(os.listdir(self.d), ["s1.json"])

    def test_prune_keeps_newest_per_scope(self):
        for sid, scope, days in [("a", X, 40), ("b", X, 35), ("c", X, 1), ("d", Y, 40)]:
            self.save(sid, scope, [rec(0)]); self.age(sid, days)
        instr.prune(self.d)
        self.assertEqual(sorted(os.listdir(self.d)), ["c.json", "d.json"])


class Fallback(Base):
    def test_newest_same_scope_copy_is_labelled_and_written(self):
        self.save("old", X, [rec(0)], "OLD"); self.age("old", 3)
        self.save("new", X, [rec(1)], "NEW"); self.age("new", 1)
        self.save("other", Y, [rec(2)], "OTHER")
        code, out = self.run_cmd("fallback", self.d, "sid", X)
        self.assertEqual(code, 0)
        self.assertEqual(out.strip(), self.doc("new")["fetched"][:10])
        doc = self.doc("sid")
        self.assertEqual((doc["opening"], doc["fallback"]), ("NEW", True))
        self.assertEqual(stat.S_IMODE(os.stat(os.path.join(self.d, "sid.json")).st_mode), 0o600)

    def test_none_exits_1_silently(self):
        self.save("other", Y, [rec(0)])
        self.assertEqual(self.run_cmd("fallback", self.d, "sid", X), (1, ""))
        self.assertFalse(os.path.exists(os.path.join(self.d, "sid.json")))


class Text(Base):
    def test_missing_copy(self):
        self.assertEqual(self.run_cmd("text", self.d, "nope", "9800"), (0, instr.UNAVAILABLE + "\n"))

    def test_no_records_prints_nothing(self):
        self.save("s1", X, [])
        self.assertEqual(self.run_cmd("text", self.d, "s1", "9800"), (0, ""))

    def test_whole_records_and_closing_line(self):
        self.save("s1", X, [rec(0, 4000), rec(1, 4000), rec(2, 4000)])
        code, out = self.run_cmd("text", self.d, "s1", "9800")
        out = out.rstrip("\n")
        self.assertLessEqual(len(out), 9800)
        self.assertIn("a" * 4000, out); self.assertIn("b" * 4000, out)
        self.assertNotIn("c" * 10, out)
        self.assertTrue(out.endswith("\n\nNot delivered, over the hook's limit: identity/preference/r2.md."))
        self.assertEqual(self.doc("s1")["omitted"], ["identity/preference/r2.md"])
        self.assertEqual(stat.S_IMODE(os.stat(os.path.join(self.d, "s1.json")).st_mode), 0o600)

    def test_everything_fits(self):
        self.save("s1", X, [rec(0), rec(1)])
        self.assertEqual(self.run_cmd("text", self.d, "s1", "9800")[1], "OPEN\n\n%s\n\n%s\n" % ("a" * 10, "b" * 10))
        self.assertEqual(self.doc("s1")["omitted"], [])

    def test_never_cuts_a_record(self):
        self.save("s1", X, [rec(0, 50), rec(1, 50)])
        for cap in range(0, 300):
            out = self.run_cmd("text", self.d, "s1", str(cap))[1].rstrip("\n")
            self.assertLessEqual(len(out), cap)
            self.assertIn(out.count("a"), (0, 50))
            self.assertIn(out.count("b"), (0, 50))

    def test_fresh_save_is_not_labelled(self):
        self.save("s1", X, [rec(0)])
        self.assertFalse(self.run_cmd("text", self.d, "s1", "9800")[1].startswith("Instructions from"))

    def test_same_day_fallback_is_labelled_and_counts_toward_cap(self):
        self.save("old", X, [rec(0)])
        self.run_cmd("fallback", self.d, "sid", X)
        day = self.doc("sid")["fetched"][:10]
        label = "Instructions from the saved copy of %s; the kernel could not be reached." % day
        out = self.run_cmd("text", self.d, "sid", "9800")[1]
        self.assertTrue(out.startswith(label + "\n\nOPEN"))
        tight = self.run_cmd("text", self.d, "sid", str(len(label) + 2 + 4 + 2 + 10))[1].rstrip("\n")
        self.assertIn("a" * 10, tight)
        self.assertEqual(self.run_cmd("text", self.d, "sid", str(len(label) + 2 + 4 + 2 + 9))[1].count("a" * 10), 0)

    def test_sessions_do_not_share(self):
        self.save("sid-a", X, [rec(0)]); self.save("sid-b", Y, [rec(1)])
        a = self.run_cmd("text", self.d, "sid-a", "9800")[1]
        self.assertIn("a" * 10, a); self.assertNotIn("b" * 10, a)

    def test_path_like_sid_rejected(self):
        with mock.patch("sys.stderr", io.StringIO()):
            self.assertEqual(self.run_cmd("text", self.d, "../x", "9800")[0], 2)


if __name__ == "__main__":
    unittest.main()

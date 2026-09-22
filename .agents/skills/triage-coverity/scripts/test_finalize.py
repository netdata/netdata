"""Exercise real Coverity verdict/payload helpers in a disposable repo; curl is offline."""

import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest

SCRIPTS = Path(__file__).resolve().parent


class FinalizeTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix="coverity-verdict-")
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.env = os.environ.copy()
        for key in ("GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE"):
            self.env.pop(key, None)
        subprocess.run(["git", "init", "-q", str(self.root)], check=True, env=self.env)
        self.scripts = self.root / "scripts"
        self.scripts.mkdir()
        for name in ("_lib.sh", "finalize-defect.sh", "update-triage.sh"):
            shutil.copy2(SCRIPTS / name, self.scripts / name)
        (self.root / ".env").write_text(
            "COVERITY_COOKIE='COVJSESSIONID-build=[TEST_SESSION]; XSRF-TOKEN=[TEST_XSRF]'\n"
            "COVERITY_PROJECT_ID=123\nCOVERITY_HOST=https://coverity.invalid\n"
        )
        self.comment = self.root / "comment with spaces.txt"
        self.comment.write_text('Source evidence: "quoted"; $(literal) `literal`.\n')
        self.capture = self.root / "request.json"
        fake_bin = self.root / "bin"
        fake_bin.mkdir()
        curl = fake_bin / "curl"
        curl.write_text(f"#!{sys.executable}\n" + '''import json, os, pathlib, sys
args = sys.argv[1:]
payload = json.loads(args[args.index("--data-raw") + 1])
pathlib.Path(os.environ["COVERITY_TEST_CAPTURE"]).write_text(json.dumps({"url": args[-1], "payload": payload}))
pathlib.Path(args[args.index("-o") + 1]).write_text('{"defectStatus":"Triaged"}')
print("200", end="")
''')
        curl.chmod(0o755)
        self.env.update(PATH=str(fake_bin) + os.pathsep + self.env["PATH"],
                        COVERITY_TEST_CAPTURE=str(self.capture))
        raw = self.root / ".local/audits/coverity/raw"
        raw.mkdir(parents=True)
        self.table = raw / "outstanding-all.json"
        self.table.write_text('[{"cid":42,"displayImpact":"High"}]')

    def run_verdict(self, verdict, sha=None, scope="outstanding"):
        if self.capture.exists():
            self.capture.unlink()
        args = ["bash", str(self.scripts / "finalize-defect.sh"), "42", verdict,
                scope, str(self.comment)]
        if sha is not None:
            args.append(sha)
        result = subprocess.run(args, cwd=self.root, env=self.env, capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertNotIn("[TEST_XSRF]", result.stdout + result.stderr)
        if not self.capture.exists():
            return None
        request = json.loads(self.capture.read_text())
        self.assertEqual(request["url"], "https://coverity.invalid/sourcebrowser/updatedefecttriage.json")
        payload = request["payload"]
        self.assertEqual(payload["mergedDefectIds"], [42])
        self.assertEqual(payload["projectId"], 123)
        self.assertEqual(payload["type"], "apply")
        self.assertEqual(payload["comment"], self.comment.read_text() +
                         (f"\nFix commit: {sha}\n" if sha else ""))
        return {row["attributeId"]: row["attributeValue"] for row in payload["triageValues"]}

    def test_unfixed_bugs_require_fix(self):
        for kind in ("MEMORY_CORRUPTION", "CRASH", "RESOURCE_LEAK", "LOGIC", "UB"):
            with self.subTest(kind=kind):
                self.assertEqual(self.run_verdict("TRUE_BUG_" + kind),
                                 {3: "24", 1: "11", 2: "2", 4: None})

    def test_submitted_fix_reference_selects_submitted_action(self):
        self.assertEqual(self.run_verdict("TRUE_BUG_LOGIC", "a" * 40),
                         {3: "24", 1: "11", 2: "3", 4: None})

    def test_empty_fix_reference_still_requires_fix(self):
        self.assertEqual(self.run_verdict("TRUE_BUG_CRASH", "")[2], "2")

    def test_existing_ignore_classifications_remain_independent(self):
        for verdict, classification in (("FALSE_POSITIVE_GUARD_EXISTS", "22"),
                                        ("COSMETIC", "23")):
            with self.subTest(verdict=verdict):
                attrs = self.run_verdict(verdict, scope="dismissed")
                self.assertEqual((attrs[3], attrs[2]), (classification, "5"))

    def test_severity_does_not_depend_on_fix_status(self):
        for impact, severity in (("Medium", "12"), ("Low", "13"), (None, "10")):
            self.table.write_text(json.dumps([{"cid": 42, "displayImpact": impact}]))
            with self.subTest(impact=impact):
                self.assertEqual(self.run_verdict("TRUE_BUG_LOGIC")[1], severity)

    def test_bookkeeping_is_local_without_authentication(self):
        (self.root / ".env").unlink()
        for verdict in ("CODE_GONE", "NEEDS_HUMAN"):
            with self.subTest(verdict=verdict):
                self.assertIsNone(self.run_verdict(verdict))


if __name__ == "__main__":
    unittest.main()

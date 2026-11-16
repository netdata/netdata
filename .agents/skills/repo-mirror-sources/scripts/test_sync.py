"""Exercise sync branch selection without real Git, mirrors, credentials, or network."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest


FAKE_GIT = '''#!/usr/bin/env python3
import json, os, sys
with open(os.environ["SYNC_TEST_LOG"], "a") as log:
    log.write(json.dumps(sys.argv[1:]) + "\\n")
args = sys.argv[1:]
cmd = args[0]
if cmd == "symbolic-ref":
    branch = os.environ["SYNC_TEST_DEFAULT"]
    if branch:
        print("refs/remotes/origin/" + branch)
    else:
        sys.exit(1)
elif cmd == "show-ref":
    sys.exit(1)
elif cmd == "rev-parse":
    if "--verify" in args:
        sys.exit(1)
    print("feature" if "--abbrev-ref" in args else "0123456789abcdef")
elif cmd == "diff-index":
    sys.exit(int(os.environ.get("SYNC_TEST_INDEX_STATUS", "0")))
elif cmd == "checkout":
    sys.exit(int(os.environ.get("SYNC_TEST_CHECKOUT_STATUS", "0")))
'''


class SyncBranchSelectionTest(unittest.TestCase):
    def run_sync(self, default="main", index_status=0, checkout_status=0):
        script = Path(__file__).with_name("sync-netdata-repos.sh").resolve()
        with tempfile.TemporaryDirectory(prefix="netdata-sync-test-") as directory:
            root = Path(directory)
            mirror = root / "mirror"
            (mirror / "sample" / ".git").mkdir(parents=True)
            commands = root / "bin"
            commands.mkdir()
            for name, source in {"git": FAKE_GIT, "jq": "#!/bin/sh\nexit 0\n", "gh": "#!/bin/sh\nexit 1\n"}.items():
                path = commands / name
                path.write_text(source)
                path.chmod(0o755)
            log = root / "commands.jsonl"
            env = {
                "PATH": str(commands) + os.pathsep + os.environ["PATH"],
                "NETDATA_REPOS_DIR": str(mirror),
                "SYNC_TEST_LOG": str(log),
                "SYNC_TEST_DEFAULT": default,
                "SYNC_TEST_INDEX_STATUS": str(index_status),
                "SYNC_TEST_CHECKOUT_STATUS": str(checkout_status),
            }
            result = subprocess.run(["bash", str(script), "--repo", "sample"], env=env, capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            calls = [json.loads(line) for line in log.read_text().splitlines()]
            return result.stdout, calls

    def test_unknown_default_does_not_update_current_branch(self):
        output, calls = self.run_sync(default="")
        self.assertFalse([args for args in calls if args[0] in {"checkout", "fetch", "pull", "submodule"}], calls)
        self.assertIn("default branch", output)

    def test_known_default_is_selected_before_pull(self):
        _, calls = self.run_sync()
        self.assertIn(["checkout", "main"], calls)
        self.assertIn(["pull", "origin", "main"], calls)
        self.assertLess(calls.index(["checkout", "main"]), calls.index(["pull", "origin", "main"]))

    def test_failed_switch_does_not_pull_feature_branch(self):
        _, calls = self.run_sync(checkout_status=1)
        self.assertFalse([args for args in calls if args[0] in {"fetch", "pull", "submodule"}], calls)

    def test_unknown_index_state_skips_repository(self):
        _, calls = self.run_sync(index_status=2)
        self.assertFalse([args for args in calls if args[0] in {"checkout", "fetch", "pull", "submodule"}], calls)


if __name__ == "__main__":
    unittest.main()

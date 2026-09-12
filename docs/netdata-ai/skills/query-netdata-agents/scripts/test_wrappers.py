"""Offline checks for authentication handling; API response bodies remain unchanged."""

import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest

SCRIPTS = Path(__file__).resolve().parent


class WrapperTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix="agents-wrapper-")
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.env = os.environ.copy()
        for key in ("GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE", "AGENTS_DRY_RUN"):
            self.env.pop(key, None)
        subprocess.run(["git", "init", "-q", str(self.root)], env=self.env, check=True)
        shutil.copy2(SCRIPTS / "_lib.sh", self.root / "_lib.sh")
        self.capture = self.root / "requests.jsonl"
        self.node = "00000000-0000-0000-0000-" + "1" * 12
        self.mg = "00000000-0000-0000-0000-" + "2" * 12
        self.bearer = "00000000-0000-0000-0000-" + "3" * 12
        fake_bin = self.root / "bin"
        fake_bin.mkdir()
        curl = fake_bin / "curl"
        curl.write_text(f"#!{sys.executable}\n" + '''import json, os, pathlib, sys, time
args = sys.argv[1:]
url = args[-1] if args[-2:-1] != ["-d"] else args[-3]
with pathlib.Path(os.environ["TEST_CAPTURE"]).open("a") as out:
    out.write(json.dumps({"url": url, "args": args}) + "\\n")
if url.endswith("/api/v3/info"):
    print(json.dumps({"agents": [{"cloud": {"claim_id": "[TEST_CLAIM]"}}]}))
elif "/bearer_get_token?" in url:
    if os.environ.get("TEST_MINT_FAILURE") == "1":
        print(json.dumps({"status": 400, "error": "TEST_CLOUD_TOKEN [TEST_CLAIM]"}))
    else:
        print(json.dumps({"token": os.environ["TEST_BEARER"], "expiration": int(time.time()) + 86400}))
else:
    if os.environ.get("TEST_DIRECT_FAILURE") == "1" and url.startswith("http:"):
        sys.exit(22)
    print(os.environ.get("TEST_RESPONSE", '{"status":200,"data":[1,2]}'))
''')
        curl.chmod(0o755)
        self.env.update(PATH=str(fake_bin) + os.pathsep + self.env["PATH"],
                        NETDATA_CLOUD_TOKEN="TEST_CLOUD_TOKEN",
                        NETDATA_CLOUD_HOSTNAME="cloud.invalid",
                        TEST_CAPTURE=str(self.capture), TEST_BEARER=self.bearer,
                        TEST_NODE=self.node, TEST_MG=self.mg)

    def run_shell(self, command):
        return subprocess.run(["bash", "-c", 'source ./_lib.sh; ' + command],
                              cwd=self.root, env=self.env, capture_output=True, text=True)

    def direct(self):
        return self.run_shell('agents_call_function --via agent --node "$TEST_NODE" '
                              '--host agent.invalid:19999 --machine-guid "$TEST_MG" '
                              '--function systemd-journal')

    def requests(self):
        return [json.loads(line) for line in self.capture.read_text().splitlines()]

    def assert_auth_hidden(self, result):
        for value in ("TEST_CLOUD_TOKEN", "[TEST_CLAIM]", self.bearer):
            self.assertNotIn(value, result.stdout + result.stderr)

    def test_cloud_response_is_unchanged_not_generically_redacted(self):
        self.env["TEST_RESPONSE"] = '{"claim_id":"[RESPONSE_CLAIM]","data":[1,2]}'
        result = self.run_shell("agents_query_cloud GET /api/v2/spaces")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads(result.stdout), json.loads(self.env["TEST_RESPONSE"]))
        self.assert_auth_hidden(result)
        self.assertIn("Authorization: Bearer " + self.env["NETDATA_CLOUD_TOKEN"], self.requests()[0]["args"])

    def test_direct_mint_and_cache_hide_authentication_values(self):
        for _ in range(2):
            result = self.direct()
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertEqual(json.loads(result.stdout), {"status": 200, "data": [1, 2]})
            self.assert_auth_hidden(result)
        requests = self.requests()
        self.assertEqual(sum("/bearer_get_token?" in r["url"] for r in requests), 1)
        self.assertIn("X-Netdata-Auth: Bearer " + self.bearer, requests[-1]["args"])
        cache = self.root / ".local/audits/query-netdata-agents/bearers" / (self.mg + ".json")
        self.assertEqual(cache.stat().st_mode & 0o777, 0o600)
        self.assertEqual(cache.parent.stat().st_mode & 0o777, 0o700)
        self.assertEqual(json.loads(cache.read_text())["token"], self.bearer)

    def test_mint_failure_does_not_echo_credential_response(self):
        self.env["TEST_MINT_FAILURE"] = "1"
        result = self.direct()
        self.assertNotEqual(result.returncode, 0)
        self.assert_auth_hidden(result)
        self.assertEqual(len(self.requests()), 2)

    def test_direct_failure_does_not_retry_through_cloud(self):
        self.env["TEST_DIRECT_FAILURE"] = "1"
        result = self.direct()
        self.assertEqual(result.returncode, 22)
        self.assert_auth_hidden(result)
        self.assertFalse(any(r["url"].startswith("https:") and "/function?" in r["url"]
                             for r in self.requests()))


if __name__ == "__main__":
    unittest.main()

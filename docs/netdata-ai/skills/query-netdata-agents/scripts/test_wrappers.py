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

    def resolve(self, caller):
        if caller == "function":
            return self.direct()
        if caller == "query":
            return self.run_shell('agents_query_agent --node "$TEST_NODE" '
                                  '--host agent.invalid:19999 --machine-guid "$TEST_MG" '
                                  'GET /api/v3/data')
        return self.run_shell('resolve_test() { local bearer; '
                              '_agents_resolve_bearer bearer "$TEST_NODE" "$TEST_MG" '
                              'agent.invalid:19999; }; resolve_test')

    def test_invalid_cache_keys_fail_before_cache_creation_or_requests(self):
        for caller in ("resolver", "query", "function"):
            for key in ("../victim", "../../victim", "/tmp/victim", "not-a-uuid",
                        self.mg + "/../victim", self.mg + "\n", self.mg[:-1],
                        "g" + self.mg[1:]):
                with self.subTest(caller=caller, key=key):
                    self.env["TEST_MG"] = key
                    result = self.resolve(caller)
                    self.assertNotEqual(result.returncode, 0)
                    self.assertEqual(result.stdout, "")
                    self.assertFalse((self.root / ".local").exists())
                    self.assertFalse(self.capture.exists())

    def test_traversal_cannot_read_overwrite_or_delete_sibling_cache(self):
        cache_dir = self.root / ".local/audits/query-netdata-agents/bearers"
        cache_dir.mkdir(parents=True)
        victim = cache_dir.parent / "victim.json"
        self.env["TEST_MG"] = "../victim"
        for caller in ("resolver", "query", "function"):
            for state in ("fresh", "expired", "mint-failure"):
                with self.subTest(caller=caller, state=state):
                    content = json.dumps({"token": "SIBLING_TOKEN",
                                          "expiration": 9999999999 if state == "fresh" else 1})
                    victim.write_text(content)
                    self.env["TEST_MINT_FAILURE"] = "1" if state == "mint-failure" else "0"
                    result = self.resolve(caller)
                    self.assertNotEqual(result.returncode, 0)
                    self.assertEqual(victim.read_text(), content)
                    self.assertFalse(self.capture.exists())
                    self.assertNotIn("SIBLING_TOKEN", result.stdout + result.stderr)

    def test_linked_cache_entry_cannot_read_overwrite_or_remove_target(self):
        cache_dir = self.root / ".local/audits/query-netdata-agents/bearers"
        cache_dir.mkdir(parents=True)
        victim = self.root / "victim.json"
        cache_file = cache_dir / (self.mg + ".json")
        cache_file.symlink_to(victim)
        for caller in ("resolver", "query", "function"):
            for state in ("fresh", "expired", "mint-failure", "missing"):
                with self.subTest(caller=caller, state=state):
                    content = json.dumps({"token": "LINKED_TOKEN",
                                          "expiration": 9999999999 if state == "fresh" else 1})
                    if state == "missing":
                        victim.unlink()
                    else:
                        victim.write_text(content)
                    self.env["TEST_MINT_FAILURE"] = "1" if state == "mint-failure" else "0"
                    result = self.resolve(caller)
                    self.assertNotEqual(result.returncode, 0)
                    self.assertTrue(cache_file.is_symlink())
                    if state == "missing":
                        self.assertFalse(victim.exists())
                    else:
                        self.assertEqual(victim.read_text(), content)
                    self.assertFalse(self.capture.exists())
                    self.assertNotIn("LINKED_TOKEN", result.stdout + result.stderr)

    def test_linked_cache_directory_is_rejected_without_chmod_or_requests(self):
        audit_dir = self.root / ".local/audits/query-netdata-agents"
        audit_dir.mkdir(parents=True)
        victim = self.root / "external-cache"
        victim.mkdir(mode=0o755)
        victim.chmod(0o755)  # Establish the permission fixture independently of the caller umask.
        (audit_dir / "bearers").symlink_to(victim, target_is_directory=True)
        for caller in ("resolver", "query", "function"):
            with self.subTest(caller=caller):
                result = self.resolve(caller)
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(list(victim.iterdir()), [])
                self.assertEqual(victim.stat().st_mode & 0o777, 0o755)
                self.assertFalse(self.capture.exists())

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

    def test_uppercase_uuid_keeps_its_cache_key(self):
        self.env["TEST_MG"] = "ABCDEF01-2345-6789-ABCD-EF0123456789"
        for _ in range(2):
            result = self.resolve("query")
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assert_auth_hidden(result)
        self.assertEqual(sum("/bearer_get_token?" in r["url"] for r in self.requests()), 1)
        cache = self.root / ".local/audits/query-netdata-agents/bearers" / (self.env["TEST_MG"] + ".json")
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

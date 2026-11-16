"""Offline contracts for the actual shell helpers; no credentials or HTTP access."""

import json
import os
from pathlib import Path
import subprocess
import shutil
import tempfile
import unittest


HELPER = Path(__file__).resolve().parents[1] / "scripts" / "_lib.sh"
WRAPPERS = {
    "codacyaudit_get": "codacyaudit_get /fixture",
    "codacyaudit_post": "codacyaudit_post /fixture '{}'",
    "codacyaudit_get_paged": "codacyaudit_get_paged /fixture",
    "codacyaudit_pr_issues": "codacyaudit_pr_issues 123",
    "codacyaudit_repo_info": "codacyaudit_repo_info",
}


class HelperTests(unittest.TestCase):
    def run_shell(self, script):
        env = {key: value for key, value in os.environ.items()
               if not key.startswith(("CODACY_", "BASH_FUNC_"))
               and key not in {"BASH_ENV", "ENV", "SHELLOPTS", "BASHOPTS"}}
        with tempfile.TemporaryDirectory() as directory:
            # A shell function and PATH guard prevent any fallback to real curl.
            guard = Path(directory) / "curl"
            guard.write_text("#!/bin/sh\necho unexpected-transport >&2\nexit 99\n")
            guard.chmod(0o700)
            env["PATH"] = directory + os.pathsep + env.get("PATH", "")
            return subprocess.run(
                ["bash", "--noprofile", "--norc", "-c", '''
source "$1"
CODACY_TOKEN=synthetic-secret
CODACY_HOST=https://fixture.invalid
CODACY_PROVIDER=fixture-provider
CODACY_ORG=fixture-org
CODACY_REPO=fixture-repo
curl() { printf 'unexpected-transport\\n' >&2; return 99; }
codacyaudit_load_env() { printf 'unexpected-env-load\\n' >&2; return 98; }
''' + script, "test-helpers", str(HELPER)],
                cwd=directory, env=env, text=True, capture_output=True, timeout=15,
            )

    @unittest.skipUnless(shutil.which("zsh"), "zsh is not installed")
    def test_documented_selftest_can_be_sourced_in_zsh(self):
        with tempfile.TemporaryDirectory() as directory:
            env = {"PATH": os.environ.get("PATH", "")}
            result = subprocess.run(
                [shutil.which("zsh"), "-f", "-c", '''
source "$1"
curl() { return 99; }
codacyaudit_load_env() { return 98; }
codacyaudit_selftest_no_token_leak
''', "test-zsh", str(HELPER)], cwd=directory, env=env,
                text=True, capture_output=True, timeout=15,
            )
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertIn("PASS", result.stdout)
            self.assertEqual(result.stderr, "")

    def test_successful_response_is_forwarded(self):
        for command in (WRAPPERS["codacyaudit_get"], WRAPPERS["codacyaudit_post"]):
            with self.subTest(command=command):
                result = self.run_shell('''
curl() { printf '%s' '{"data":"fixture response"}'; }
''' + command)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(result.stdout, '{"data":"fixture response"}')
                self.assertEqual(result.stderr, "")

    def test_failed_transport_preserves_status_without_exposing_output(self):
        for command in WRAPPERS.values():
            with self.subTest(command=command):
                result = self.run_shell('''
curl() {
    printf '%s private-response-body' "$CODACY_TOKEN"
    printf '%s private-transport-error' "$CODACY_TOKEN" >&2
    return 23
}
''' + command)
                self.assertEqual(result.returncode, 23, result.stderr)
                self.assertEqual(result.stdout, "")
                self.assertTrue(result.stderr, "Failure must report a diagnostic")
                for secret in ("synthetic-secret", "private-response-body", "private-transport-error"):
                    self.assertNotIn(secret, result.stdout + result.stderr)

    def test_paginated_response_combines_pages_and_preserves_query(self):
        result = self.run_shell('''
curl() {
    local url="${@: -1}"
    case "$url" in
        'https://fixture.invalid/fixture?filter=open&limit=2')
            printf '%s' '{"data":[{"id":1}],"pagination":{"cursor":"page-two"}}' ;;
        'https://fixture.invalid/fixture?filter=open&limit=2&cursor=page-two')
            printf '%s' '{"data":[{"id":2}],"pagination":{}}' ;;
        *) printf 'unexpected pagination request' >&2; return 97 ;;
    esac
}
codacyaudit_get_paged '/fixture?filter=open' 2
''')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads(result.stdout), [{"id": 1}, {"id": 2}])
        self.assertEqual(result.stderr, "")

    def test_later_page_failure_preserves_status_without_partial_results(self):
        result = self.run_shell('''
curl() {
    case "${@: -1}" in
        *cursor=second) return 23 ;;
        *) printf '%s' '{"data":[{"id":1}],"pagination":{"cursor":"second"}}' ;;
    esac
}
codacyaudit_get_paged /fixture
''')
        self.assertEqual(result.returncode, 23, result.stderr)
        self.assertEqual(result.stdout, "")

    def test_selftest_is_offline_and_preserves_caller_state(self):
        for setup in (
            "unset CODACY_TOKEN CODACY_HOST CODACY_PROVIDER CODACY_ORG CODACY_REPO",
            "export CODACY_TOKEN CODACY_HOST; export -n CODACY_PROVIDER CODACY_ORG CODACY_REPO",
            "export -n CODACY_TOKEN CODACY_HOST; export CODACY_PROVIDER CODACY_ORG CODACY_REPO",
        ):
            with self.subTest(setup=setup):
                result = self.run_shell(setup + '''
before="$(declare -p CODACY_TOKEN CODACY_HOST CODACY_PROVIDER CODACY_ORG CODACY_REPO 2>/dev/null || true)"
curl() { printf 'called\\n' >> transport-calls; return 99; }
codacyaudit_load_env() { printf 'called\\n' >> env-calls; return 98; }
codacyaudit_selftest_no_token_leak
after="$(declare -p CODACY_TOKEN CODACY_HOST CODACY_PROVIDER CODACY_ORG CODACY_REPO 2>/dev/null || true)"
[[ "$before" == "$after" ]] || { printf 'caller-state-changed' >&2; exit 96; }
[[ ! -e transport-calls && ! -e env-calls ]] || { printf 'external-access' >&2; exit 95; }
''')
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertIn("PASS", result.stdout)
                self.assertEqual(result.stderr, "")

    def test_selftest_rejects_each_wrapper_stderr_leak(self):
        for wrapper in WRAPPERS:
            with self.subTest(wrapper=wrapper):
                result = self.run_shell(wrapper + '''() {
    printf '%s' "$CODACY_TOKEN" >&2
    printf '%s' '{"fixture":"leak-test"}'
}
codacyaudit_selftest_no_token_leak
''')
                self.assertNotEqual(result.returncode, 0)
                self.assertNotIn("PASS", result.stdout)

    def test_selftest_rejects_each_wrapper_failure(self):
        for wrapper in WRAPPERS:
            with self.subTest(wrapper=wrapper):
                result = self.run_shell(wrapper + '''() { return 23; }
codacyaudit_selftest_no_token_leak
''')
                self.assertNotEqual(result.returncode, 0)
                self.assertNotIn("PASS", result.stdout)


if __name__ == "__main__":
    unittest.main()

#!/usr/bin/env python3
"""Offline observable-contract tests for agent-events shell helpers."""

import json
import os
from pathlib import Path
import shlex
import shutil
import stat
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[4]
SCRIPTS = ROOT / '.agents/skills/triage-agent-events/scripts'
BASH = '/opt/homebrew/bin/bash' if Path('/opt/homebrew/bin/bash').exists() else shutil.which('bash')


@unittest.skipUnless(BASH and shutil.which('jq'), 'bash and jq are required')
class HelpersTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='agent-events-tests-')
        self.addCleanup(self.temp.cleanup)
        self.work = Path(self.temp.name)
        self.calls = self.work / 'calls.jsonl'
        self.fixture = self.work / 'fixture.json'
        self.audit = self.work / 'audit'
        self.audit.mkdir()
        self.blocked = self.work / 'blocked'
        self.env = {
            'PATH': os.environ.get('PATH', '/usr/bin:/bin'),
            'TMPDIR': str(self.work),
            'TEST_CALLS': str(self.calls),
            'TEST_FIXTURE': str(self.fixture),
            'TEST_AUDIT': str(self.audit),
            'TEST_BLOCKED': str(self.blocked),
            'NETDATA_CLOUD_TOKEN': 'synthetic-outer-token',
            'NETDATA_CLOUD_HOSTNAME': 'cloud.invalid',
            'AGENT_EVENTS_HOSTNAME': 'agent.invalid',
            'AGENT_EVENTS_MACHINE_GUID': '11111111-2222-3333-4444-555555555555',
            'AGENT_EVENTS_NODE_ID': '22222222-3333-4444-5555-666666666666',
        }
        self.source = 'source ' + shlex.quote(str(SCRIPTS / '_lib.sh')) + '\n'
        self.guards = r'''
# Every test rejects external transport and environment loading.
curl() { printf 'curl\n' >> "$TEST_BLOCKED"; return 97; }
agents_load_env() { printf 'env\n' >> "$TEST_BLOCKED"; return 97; }
agentevents_load_env() { printf 'env\n' >> "$TEST_BLOCKED"; return 97; }
_agents_resolve_bearer() { printf 'bearer\n' >> "$TEST_BLOCKED"; return 97; }
'''
        self.recorder = r'''
agentevents_query_function() {
    jq -nc --arg via "$1" --argjson payload "$2" '{via:$via,payload:$payload}' >> "$TEST_CALLS"
    cat "$TEST_FIXTURE"
    return "${TEST_TRANSPORT_STATUS:-0}"
}
agentevents_audit_dir() { printf '%s\n' "$TEST_AUDIT"; }
'''
        self.set_versions(['v2.10.0'])

    def run_shell(self, code):
        return subprocess.run([BASH, '-c', self.source + self.guards + code],
                              cwd=ROOT, env=self.env, text=True, capture_output=True, timeout=15)

    def assert_success(self, result):
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse(self.blocked.exists(), 'external transport or environment loading attempted')

    def set_versions(self, versions):
        self.fixture.write_text(json.dumps({
            'facets': [{'id': 'AE_AGENT_VERSION', 'options': [{'id': v} for v in versions]}],
            'columns': {'AE_AGENT_VERSION': {'index': 0}},
            'data': [['v2.10.0'], ['v2.9.0']],
        }))

    def requests(self):
        return [json.loads(line) for line in self.calls.read_text().splitlines()]

    def compute(self, *args):
        result = self.run_shell(self.recorder + '\nagentevents_compute_default_versions ' + shlex.join(args))
        self.assert_success(result)
        return json.loads(result.stdout)

    def cli(self, *args, relative_output=False):
        # Only the sibling library changes: run a byte-identical copy of the CLI.
        cli = self.work / 'get-events.sh'
        shutil.copyfile(SCRIPTS / 'get-events.sh', cli)
        change_directory = '\ncd -- ' + shlex.quote(str(self.work)) + '\n' if relative_output else ''
        (self.work / '_lib.sh').write_text(self.source + self.guards + self.recorder + '\nagentevents_load_env() { :; }\n' + change_directory)
        return subprocess.run([BASH, str(cli), *args], cwd=ROOT, env=self.env,
                              text=True, capture_output=True, timeout=15)

    def test_stable_versions_sort_numeric_components(self):
        self.set_versions(['v2.9.99', 'v2.10.0', 'v2.10.1', 'v1.99.99', 'junk'])
        self.assertEqual(self.compute(), ['v2.10.1'])

    def test_nightlies_sort_base_then_commit_count(self):
        self.set_versions(['v2.9.0-999-nightly', 'v2.10.0-2-nightly',
                           'v2.10.0-10-nightly', 'v2.11.0-1-nightly',
                           'v2.10.0', 'v2.11.0-1-custom'])
        self.assertEqual(self.compute(), ['v2.10.0', 'v2.11.0-1-nightly',
                                         'v2.10.0-10-nightly', 'v2.10.0-2-nightly'])

    def test_versions_use_only_observed_values(self):
        self.set_versions(['v2.10.0-2-nightly', 'unknown'])
        self.assertEqual(self.compute(), ['v2.10.0-2-nightly'])
        self.set_versions([])
        self.assertEqual(self.compute(), [])

    def test_discovery_defaults(self):
        self.compute()
        request = self.requests()[0]
        self.assertEqual(request['via'], 'cloud')
        self.assertEqual((request['payload']['after'], request['payload']['before']), (-86400, 0))

    def test_discovery_preserves_historical_window(self):
        self.compute('agent', '-172800', '-86400')
        request = self.requests()[0]
        self.assertEqual(request['via'], 'agent')
        self.assertEqual((request['payload']['after'], request['payload']['before']), (-172800, -86400))

    def test_discovery_preserves_absolute_window(self):
        self.compute('cloud', '1700000000', '1700086400')
        payload = self.requests()[0]['payload']
        self.assertEqual((payload['after'], payload['before']), (1700000000, 1700086400))

    def test_cli_discovery_and_fetch_use_identical_window(self):
        for after, before in [('48h ago', '-86400'), ('1700000000', '1700086400')]:
            with self.subTest(after=after):
                result = self.cli('--via', 'agent', '--since', after, '--before', before)
                self.assert_success(result)
                discovery, fetch = self.requests()[-2:]
                self.assertEqual(discovery['via'], fetch['via'])
                for key in ('after', 'before'):
                    self.assertEqual(discovery['payload'][key], fetch['payload'][key])

    def test_empty_auto_versions_stop_before_fetch(self):
        self.set_versions([])
        result = self.cli()
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.blocked.exists())
        self.assertEqual(len(self.requests()), 1, 'empty discovery must not widen the main fetch')
        self.assertEqual(list(self.audit.iterdir()), [])

    def test_explicit_versions_bypass_discovery(self):
        result = self.cli('--versions', 'v2.9.0,v2.10.0')
        self.assert_success(result)
        calls = self.requests()
        self.assertEqual(len(calls), 1)
        self.assertEqual(calls[0]['payload']['selections']['AE_AGENT_VERSION'], ['v2.9.0', 'v2.10.0'])

    def test_default_outputs_are_private_and_unique(self):
        # Fix wall-clock formatting to force a collision in timestamp-only naming.
        fake_bin = self.work / 'bin'
        fake_bin.mkdir()
        date = fake_bin / 'date'
        date.write_text('#!/bin/sh\nprintf "20260101T000000Z\\n"\n')
        date.chmod(0o755)
        self.env['PATH'] = str(fake_bin) + ':' + self.env['PATH']
        paths = []
        for _ in range(2):
            result = self.cli('--versions', 'v2.10.0')
            self.assert_success(result)
            output = Path(result.stdout.strip())
            self.assertEqual(output.parent, self.audit)
            self.assertEqual(stat.S_IMODE(output.stat().st_mode), 0o600)
            paths.append(output)
        self.assertNotEqual(*paths)

    def test_existing_output_and_tmp_are_preserved(self):
        output = self.work / 'existing.json'
        sibling = self.work / 'existing.json.tmp'
        output.write_text('preserve output')
        sibling.write_text('preserve sibling')
        result = self.cli('--output', str(output), '--version', '^v2')
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(output.read_text(), 'preserve output')
        self.assertEqual(sibling.read_text(), 'preserve sibling')

    def test_leading_hyphen_output_names_are_paths(self):
        for index, selection in enumerate((('--version', '^v2[.]10[.]'),
                                            ('--versions', 'v2.10.0'))):
            with self.subTest(selection=selection):
                name = f'-events-{index}.json'
                result = self.cli('--output', name, *selection, relative_output=True)
                self.assert_success(result)
                self.assertEqual(result.stdout.strip(), './' + name)
                output = self.work / name
                rows = [['v2.10.0']] if index == 0 else [['v2.10.0'], ['v2.9.0']]
                self.assertEqual(json.loads(output.read_text())['data'], rows)
                self.assertIn(f'wrote {len(rows)} row(s)', result.stderr)
                self.assertEqual(stat.S_IMODE(output.stat().st_mode), 0o600)
                self.assertEqual(list(self.work.glob(name + '.filtered.*')), [])

    def test_regex_output_does_not_replace_existing_tmp(self):
        output = self.work / 'new.json'
        sibling = self.work / 'new.json.tmp'
        sibling.write_text('preserve sibling')
        result = self.cli('--output', str(output), '--version', '^v2[.]10[.]')
        self.assert_success(result)
        self.assertTrue(sibling.exists(), 'pre-existing .tmp sibling was removed')
        self.assertEqual(sibling.read_text(), 'preserve sibling')
        self.assertEqual(json.loads(output.read_text())['data'], [['v2.10.0']])
        self.assertEqual(stat.S_IMODE(output.stat().st_mode), 0o600)

    def test_existing_nonregular_output_and_symlink_are_rejected(self):
        fifo = self.work / 'existing.fifo'
        os.mkfifo(fifo)
        link = self.work / 'fifo-link'
        link.symlink_to(fifo)
        for output in (fifo, link):
            with self.subTest(output=output.name):
                result = self.cli('--output', str(output), '--versions', 'v2.10.0')
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse(self.calls.exists(), 'existing output should reject before fetch')
        self.assertTrue(stat.S_ISFIFO(fifo.stat().st_mode))
        self.assertTrue(link.is_symlink())

    def test_invalid_regex_preserves_raw_output_and_unrelated_sibling(self):
        output = self.work / 'regex.json'
        sibling = self.work / 'regex.json.tmp'
        sibling.write_text('preserve')
        result = self.cli('--output', str(output), '--version', '[')
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(json.loads(output.read_text()), json.loads(self.fixture.read_text()))
        self.assertEqual(sibling.read_text(), 'preserve')
        self.assertEqual(stat.S_IMODE(output.stat().st_mode), 0o600)

    def test_failed_fetch_keeps_private_partial_output(self):
        self.env['TEST_TRANSPORT_STATUS'] = '23'
        output = self.work / 'failed.json'
        result = self.cli('--output', str(output), '--versions', 'v2.10.0')
        self.assertEqual(result.returncode, 23)
        self.assertEqual(output.read_text(), self.fixture.read_text())
        self.assertEqual(stat.S_IMODE(output.stat().st_mode), 0o600)
        self.assertNotIn('wrote', result.stderr)

    def test_selftest_is_offline_and_preserves_caller_state(self):
        result = self.run_shell(r'''
unset NETDATA_CLOUD_TOKEN AGENT_EVENTS_NODE_ID
AGENTS_DRY_RUN=1
before_vars=$(declare -p NETDATA_CLOUD_HOSTNAME AGENT_EVENTS_HOSTNAME AGENT_EVENTS_MACHINE_GUID AGENTS_DRY_RUN)
before_functions=$(declare -f curl agents_load_env agentevents_load_env _agents_resolve_bearer)
before_flags=$-
before_umask=$(umask)
agentevents_selftest_no_token_leak
[[ ! ${NETDATA_CLOUD_TOKEN+x} && ! ${AGENT_EVENTS_NODE_ID+x} ]]
[[ "$before_vars" == "$(declare -p NETDATA_CLOUD_HOSTNAME AGENT_EVENTS_HOSTNAME AGENT_EVENTS_MACHINE_GUID AGENTS_DRY_RUN)" ]]
[[ "$before_functions" == "$(declare -f curl agents_load_env agentevents_load_env _agents_resolve_bearer)" ]]
[[ "$before_flags" == "$-" && "$before_umask" == "$(umask)" ]]
''')
        self.assert_success(result)
        self.assertIn('PASS', result.stdout + result.stderr)

    def test_selftest_rejects_dispatch_failure(self):
        for via in ('cloud', 'agent'):
            with self.subTest(via=via):
                result = self.run_shell(r'''
eval "$(declare -f agentevents_query_function | sed '1s/agentevents_query_function/test_original_dispatch/')"
agentevents_query_function() {
    if [[ "$1" == ''' + shlex.quote(via) + r''' ]]; then return 23; fi
    test_original_dispatch "$@"
}
agentevents_selftest_no_token_leak
''')
                self.assertNotEqual(result.returncode, 0)

    def test_selftest_rejects_missing_response(self):
        result = self.run_shell('agentevents_query_function() { :; }\nagentevents_selftest_no_token_leak\n')
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.blocked.exists())

    def test_selftest_rejects_token_leak_on_either_stream(self):
        for via in ('cloud', 'agent'):
            for stream in (1, 2):
                with self.subTest(via=via, stream=stream):
                    result = self.run_shell(r'''
eval "$(declare -f agentevents_query_function | sed '1s/agentevents_query_function/test_original_dispatch/')"
agentevents_query_function() {
    test_original_dispatch "$@" || return
    if [[ "$1" == ''' + shlex.quote(via) + r''' ]]; then
        printf '%s\n' "$NETDATA_CLOUD_TOKEN" >&''' + str(stream) + r'''
    fi
}
agentevents_selftest_no_token_leak
''')
                    self.assertNotEqual(result.returncode, 0)
                    self.assertNotIn('PASS', result.stdout + result.stderr)


if __name__ == '__main__':
    unittest.main()

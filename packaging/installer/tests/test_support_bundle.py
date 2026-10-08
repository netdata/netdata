# SPDX-License-Identifier: GPL-3.0-or-later
"""Unix support-bundle contracts against private synthetic fixtures."""
import json
import os
from pathlib import Path
import shlex
import subprocess
import tempfile
import unittest

SCRIPT = Path(__file__).resolve().parents[1] / 'netdata-support-bundle'


class SupportBundleTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        (self.root / 'bin').mkdir()
        self.env = dict(os.environ, SUPPORT_BUNDLE_SCRIPT=str(SCRIPT), FIXTURE=str(self.root),
                        ND_SUPPORT_BUNDLE_DEMOTED='1', TMPDIR=str(self.root))
        self.env['PATH'] = str(self.root / 'bin') + os.pathsep + self.env['PATH']

    def run_fixture(self, body, timeout=30, expected=0):
        setup = '''
set -u
ND_SUPPORT_BUNDLE_SOURCE_ONLY=1 . "$SUPPORT_BUNDLE_SCRIPT"
init_defaults
init_staging
detect_timeout
HOST_SHORT=""; HOST_FQDN=""; RUN_USER=""
WORK="$FIXTURE/work"
mkdir -p "$WORK"
NETDATA_PID=""; api_ok=0; IS_CONTAINER=0
'''
        shell = shlex.split(os.environ.get('SUPPORT_BUNDLE_TEST_SHELL', 'sh'))
        result = subprocess.run(shell + ['-c', setup + body], env=self.env,
                                text=True, capture_output=True, timeout=timeout)
        self.assertEqual(result.returncode, expected, result.stdout + result.stderr)
        return result

    def stub(self, name, body):
        path = self.root / 'bin' / name
        path.write_text('#!/bin/sh\n' + body)
        path.chmod(0o755)

    def test_source_does_not_collect_or_initialize(self):
        result = subprocess.run(['sh', '-c', 'ND_SUPPORT_BUNDLE_SOURCE_ONLY=1 . "$SUPPORT_BUNDLE_SCRIPT"'],
                                env=self.env, capture_output=True, text=True, timeout=5)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(list(self.root.iterdir()), [self.root / 'bin'])

    def test_failed_raw_capture_withholds_partial_json(self):
        self.run_fixture('''
collect_cmd_raw partial.json fixture sh -c 'printf "{\\"partial\\":"; exit 7'
write_manifest
''')
        body = json.loads((self.root / 'work/partial.json').read_text())
        self.assertIn('error', body)
        self.assertNotIn('partial', body)
        self.assertEqual(body['exit_code'], '7')
        manifest = json.loads((self.root / 'work/MANIFEST.json').read_text())
        self.assertEqual(manifest['files'][0]['bytes'], (self.root / 'work/partial.json').stat().st_size)

    def test_raw_timeout_withholds_partial_json(self):
        self.run_fixture('''
CMD_TIMEOUT=1
collect_cmd_raw timeout.json fixture sh -c 'printf "{\\"partial\\":"; sleep 3'
''', timeout=10)
        body = json.loads((self.root / 'work/timeout.json').read_text())
        self.assertIn('error', body)
        self.assertNotEqual(body['exit_code'], '0')

    def test_raw_overflow_and_empty_output(self):
        (self.root / 'large.json').write_text(json.dumps({'data': 'x' * 200}))
        self.run_fixture('''
API_CAP=64
collect_cmd_raw large.json fixture cat "$FIXTURE/large.json"
collect_cmd_raw empty.json fixture true
''')
        self.assertIn('cap', json.loads((self.root / 'work/large.json').read_text())['error'])
        self.assertFalse((self.root / 'work/empty.json').exists())

    def test_manifest_escapes_control_characters(self):
        value = 'line\rname\x01\b\f\\".conf'
        self.env['ODD_NAME'] = value
        self.run_fixture('''
printf 'fixture\n' > "$WORK/$ODD_NAME"
manifest_add "$ODD_NAME" file "$ODD_NAME" "$ODD_NAME"
write_manifest
''')
        entry = json.loads((self.root / 'work/MANIFEST.json').read_text())['files'][0]
        for key in ('path', 'origin', 'title'):
            self.assertEqual(entry[key], value)

    def test_command_exit_status_and_sanitization(self):
        self.run_fixture('''
collect_cmd cmd.txt fixture sh -c 'echo password=SENTINEL; exit 9'
''')
        text = (self.root / 'work/cmd.txt').read_text()
        self.assertIn('# exit: 9', text)
        self.assertNotIn('SENTINEL', text)

    def test_api_bypasses_proxy_environment(self):
        self.stub('curl', '''
printf '%s\\n' "$*" > "$FIXTURE/curl-args"
printf '%s|%s|%s|%s' "${http_proxy:-}" "${HTTP_PROXY:-}" "${all_proxy:-}" "${ALL_PROXY:-}" > "$FIXTURE/proxy-env"
printf '{"keep":true}\\n'
''')
        for key in ('http_proxy', 'HTTP_PROXY', 'all_proxy', 'ALL_PROXY'):
            self.env[key] = 'http://proxy.example.test:8080'
        self.run_fixture('collect_api api.json fixture /api/v3/info\n')
        self.assertEqual((self.root / 'proxy-env').read_text(), '|||')
        self.assertIn('--noproxy *', (self.root / 'curl-args').read_text())
        self.assertTrue(json.loads((self.root / 'work/api.json').read_text())['keep'])

    def test_api_failure_withholds_partial_body(self):
        self.stub('curl', 'printf "{\\"partial\\":"; exit 28\n')
        self.run_fixture('collect_api api.json fixture /api/v3/info\n')
        self.assertIn('error', json.loads((self.root / 'work/api.json').read_text()))

    def test_user_mapping_bound(self):
        (self.root / 'users').write_text(''.join(f'/home/account-{n}/file\n' for n in range(4100)))
        self.run_fixture('''
sanitize_file "$FIXTURE/users"
cp "$MAP_FILE" "$FIXTURE/map"
''')
        mappings = (self.root / 'map').read_text().splitlines()
        self.assertEqual(sum(line.startswith('user\t') for line in mappings), 4096)
        self.assertIn('/home/redacted-user-overflow/file', (self.root / 'users').read_text())

    def test_seed_overflow_keeps_names_for_later_captures(self):
        (self.root / 'nodes.json').write_text(json.dumps([
            {'hostname': f'node-{n}.example.test'} for n in range(4100)]))
        (self.root / 'evidence.log').write_text('node-4095.example.test node-4096.example.test node-4099.example.test\n')
        self.stub('curl', 'cat "$FIXTURE/nodes.json"\n')
        self.run_fixture('''
seed_hostnames
collect_file evidence.log fixture "$FIXTURE/evidence.log"
cp "$MAP_FILE" "$FIXTURE/map"
''')
        mappings = (self.root / 'map').read_text().splitlines()
        self.assertEqual(len(mappings), 4100)
        self.assertEqual(sum('\tprivate-host-' in line for line in mappings), 4096)
        evidence = (self.root / 'work/evidence.log').read_text()
        self.assertNotIn('.example.test', evidence)
        self.assertIn('private-host-4096', evidence)
        self.assertEqual(evidence.count('redacted-host-overflow'), 2)

    def test_large_discovery_response_keeps_hostname_knowledge(self):
        (self.root / 'nodes.json').write_text(json.dumps({
            'hostname': 'large-parent.example.test', 'data': 'x' * (2 * 1024 * 1024)}))
        (self.root / 'evidence.log').write_text('large-parent.example.test\n')
        self.stub('curl', 'cat "$FIXTURE/nodes.json"\n')
        self.run_fixture('''
seed_hostnames
collect_file evidence.log fixture "$FIXTURE/evidence.log"
collect_api api.json fixture /api/v2/node_instances
''')
        self.assertNotIn('large-parent.example.test', (self.root / 'work/evidence.log').read_text())
        self.assertIn('cap', json.loads((self.root / 'work/api.json').read_text())['error'])

    def test_failed_seed_response_is_not_used(self):
        self.stub('curl', 'printf \'{"hostname":"incomplete.example.test"}\'; exit 28\n')
        self.run_fixture('seed_hostnames\ncp "$MAP_FILE" "$FIXTURE/map"\n')
        self.assertEqual((self.root / 'map').read_text(), '')

    def test_no_obfuscation_skips_discovery_but_redacts_secrets(self):
        self.stub('curl', 'touch "$FIXTURE/discovery-called"; exit 1\n')
        (self.root / 'config').write_text('password=SENTINEL\nnode.example.test\n')
        self.run_fixture('''
OBFUSCATE=0
seed_hostnames
collect_file config fixture "$FIXTURE/config"
''')
        self.assertFalse((self.root / 'discovery-called').exists())
        result = (self.root / 'work/config').read_text()
        self.assertNotIn('SENTINEL', result)
        self.assertIn('node.example.test', result)

    def test_archive_pipeline_propagates_tar_failure(self):
        self.stub('tar', 'case "$*" in *--zstd*|*--owner*) exit 1;; esac\nprintf partial; exit 9\n')
        self.stub('zstd', 'while [ "$#" -gt 0 ]; do if [ "$1" = -o ]; then shift; cat > "$1"; exit 0; fi; shift; done\n')
        self.run_fixture('OUTDIR="$FIXTURE/output"\npublish_bundle\n', expected=1)
        self.assertEqual(list((self.root / 'output').iterdir()), [])

    def test_hostname_boundaries_and_cross_file_stability(self):
        names = ['node.example.test', 'node-2', 'dot.name', 'strange_name.example.test']
        mapping = ''.join(f'fqdn\t{name}\tprivate-host-{i+1}\n' for i, name in enumerate(names))
        (self.root / 'seed').write_text(mapping)
        sample = ('node.example.test:19999 node.example.test.extra xnode.example.test\n'
                  'node-2 node-20 /dot.name/path strange_name.example.test\n')
        for name in ('first', 'second'):
            (self.root / name).write_text(sample)
        self.run_fixture('''
cat "$FIXTURE/seed" > "$MAP_FILE"
sanitize_file "$FIXTURE/first"
sanitize_file "$FIXTURE/second"
''')
        first = (self.root / 'first').read_text()
        self.assertEqual(first, (self.root / 'second').read_text())
        self.assertEqual(first, 'private-host-1:19999 node.example.test.extra xnode.example.test\n'
                               'private-host-2 node-20 /private-host-3/path private-host-4\n')

    def test_docker_log_note_requires_review_and_uses_requested_window(self):
        log = self.root / 'logs'
        log.mkdir()
        (log / 'daemon.log').symlink_to('/dev/stdout')
        self.stub('journalctl', 'exit 0\n')
        self.stub('coredumpctl', 'exit 0\n')
        self.run_fixture('LOGDIR="$FIXTURE/logs"; SINCE_HOURS=7\ncollect_logs\n')
        note = (self.root / 'work/05-logs/LOGS-ARE-IN-DOCKER.txt').read_text()
        self.assertIn('--since 7h', note)
        self.assertIn('UNSANITIZED', note)
        self.assertIn('review', note)

    def test_state_inventory_does_not_publish_filenames(self):
        lib = self.root / 'lib'
        lib.mkdir()
        (lib / 'private-job-SENTINEL').write_text('secret bytes')
        self.run_fixture('''
LIBDIR="$FIXTURE/lib"; CACHEDIR=""; CONFDIR=""
collect_state
''')
        inventory = (self.root / 'work/06-state/state-tree.txt').read_text()
        self.assertNotIn('SENTINEL', inventory)
        self.assertIn('files: 1\n', inventory)
        self.assertIn('logical bytes: 12\n', inventory)


if __name__ == '__main__':
    unittest.main()

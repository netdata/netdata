#!/usr/bin/env python3
"""Exercise package retries without installing packages or using the network."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

SCRIPT = Path(__file__).with_name('ci-package.sh').resolve()


class PackageRetryTests(unittest.TestCase):
    def run_package(self, manager, output='', failures=0, status=1, deadline=False, args=('mod', 'download'), stderr=''):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            command = root / manager
            command.write_text('''#!/usr/bin/env python3
import os, pathlib, sys, time
p = pathlib.Path(os.environ['COUNT'])
n = int(p.read_text()) + 1 if p.exists() else 1
p.write_text(str(n))
(p.parent / 'args').write_text('\\n'.join(sys.argv[1:]))
(p.parent / 'env').write_text(os.environ.get('PIP_RETRIES', '') + ':' + os.environ.get('PIP_TIMEOUT', ''))
(p.parent / 'cargo-env').write_text(os.environ.get('CARGO_NET_RETRY', '') + ':' + os.environ.get('CARGO_HTTP_TIMEOUT', ''))
if os.environ['HANG'] == '1': time.sleep(10)
print(os.environ['OUTPUT'])
print(os.environ['STDERR'], file=sys.stderr)
sys.exit(int(os.environ['STATUS']) if n <= int(os.environ['FAILURES']) else 0)
''')
            command.chmod(0o755)
            sleep = root / 'sleep'
            sleep.write_text('#!/bin/sh\nexit 0\n')
            sleep.chmod(0o755)
            if deadline:
                timeout = root / 'timeout'
                timeout.write_text('#!/bin/sh\nshift 3\nexec /usr/bin/timeout -s KILL 0.2 "$@"\n')
                timeout.chmod(0o755)
            env = dict(os.environ, PATH=f'{root}:{os.environ["PATH"]}',
                       COUNT=str(root / 'count'), OUTPUT=output, STDERR=stderr,
                       FAILURES=str(failures), STATUS=str(status), HANG=str(int(deadline)))
            result = subprocess.run(['sh', str(SCRIPT), manager, *args],
                                    env=env, capture_output=True, text=True, timeout=5)
            self.last_args = (root / 'args').read_text().splitlines()
            self.last_result = result
            self.last_env = (root / 'env').read_text()
            self.last_cargo_env = (root / 'cargo-env').read_text()
            return result.returncode, int((root / 'count').read_text())

    def test_success(self):
        self.assertEqual(self.run_package('go'), (0, 1))

    def test_native_apt_retries_and_strict_update(self):
        self.assertEqual(self.run_package('apt-get', failures=10, status=100), (100, 1))
        self.assertIn('Acquire::Retries=5', self.last_args)
        self.assertIn('APT::Update::Error-Mode=any', self.last_args)

    def test_native_pip_retries(self):
        self.assertEqual(self.run_package('pip', failures=10), (1, 1))
        self.assertEqual(self.last_env, '5:30')

    def test_native_cargo_retries(self):
        self.assertEqual(self.run_package('cargo', failures=10), (1, 1))
        self.assertEqual(self.last_cargo_env, '5:30')

    def test_go_network_recovery(self):
        output = json.dumps({'Error': 'dial tcp: i/o timeout'})
        self.assertEqual(self.run_package('go', output, failures=2), (0, 3))

    def test_go_mixed_failure_is_not_retried(self):
        output = '\n'.join(json.dumps({'Error': error}) for error in
                           ['dial tcp: i/o timeout', 'invalid version: unknown revision'])
        self.assertEqual(self.run_package('go', output, failures=10), (1, 1))

    def test_go_stderr_does_not_contaminate_json(self):
        output = json.dumps({'Error': 'dial tcp: i/o timeout'})
        diagnostic = 'go: downloading example.invalid/module v1.0.0'
        self.assertEqual(self.run_package('go', output, failures=2, stderr=diagnostic), (0, 3))
        self.assertEqual(self.last_result.stdout, (output + '\n') * 3)
        self.assertEqual(self.last_result.stderr.count(diagnostic + '\n'), 3)
        self.assertNotIn('JSONDecodeError', self.last_result.stderr)

    def test_go_unknown_error_is_not_retried(self):
        self.assertEqual(self.run_package('go', '{"Error":"invalid version"}', 10, 7), (7, 1))

    def test_apk_network_exhaustion(self):
        self.assertEqual(self.run_package('apk', 'ERROR: package: temporary error (try again later)', 10), (1, 5))

    def test_apk_index_network_recovery(self):
        output = ('WARNING: fetching https://example.invalid/APKINDEX.tar.gz: '
                  'temporary error (try again later)\n'
                  'ERROR: 2 unavailable, 0 stale; 0 distinct packages available')
        self.assertEqual(self.run_package('apk', output, 2, args=('update',)), (0, 3))

    def test_apk_mixed_failure_is_not_retried(self):
        output = 'ERROR: package: temporary error (try again later)\nERROR: unable to select packages:'
        self.assertEqual(self.run_package('apk', output, 10), (1, 1))

    def test_deadline_terminates_command(self):
        status, attempts = self.run_package('apk', deadline=True)
        self.assertNotEqual(status, 0)
        self.assertEqual(attempts, 1)


if __name__ == '__main__':
    unittest.main()

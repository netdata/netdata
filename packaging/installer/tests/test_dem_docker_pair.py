# SPDX-License-Identifier: GPL-3.0-or-later
"""Exercise the real image adapter with private Agent/CLI paths, without Docker."""
import json
import os
from pathlib import Path
import pwd
import shlex
import subprocess
import sys
import tempfile
import unittest


ADAPTER = Path(__file__).resolve().parents[2] / 'docker/netdata-dem-docker'


class DockerPairAdapter(unittest.TestCase):
    def setUp(self):
        self.account = pwd.getpwuid(os.geteuid()) if os.geteuid() else pwd.getpwnam('nobody')
        if self.account.pw_gid == 0:
            self.skipTest('requires a nonroot UID and GID')
        self.temporary = tempfile.TemporaryDirectory(prefix='dem-docker-pair-')
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name).resolve()
        self.root.chmod(0o755)
        self.runtime = self.root / 'runtime'
        self.runtime.mkdir()
        self.calls = self.root / 'calls'
        self.calls.touch(mode=0o666)
        self.calls.chmod(0o666)
        self.history = self.root / 'history'
        self.history.write_bytes(b'preserve history\x00\xff')
        self.agent = self.root / 'agent'
        self.agent.write_text('#!/bin/sh\ncase "$6" in\n'
                              f'plugins) printf \'"%s" "%s/second"\\n\' "{self.root}" "{self.root}" ;;\n'
                              f'*) printf "%s\\n" "{self.root}" ;;\nesac\n')
        self.plugin = self.root / 'dem.plugin'
        self.plugin.write_text(f'#!{sys.executable}\n' + '''import json, os, sys
from pathlib import Path
with Path(os.environ['FIXTURE_CALLS']).open('a') as stream:
    stream.write(json.dumps(sys.argv[1:]) + '\\n')
if sys.argv[2] == 'setup-plan':
    print(os.environ['FIXTURE_PLAN'])
else:
    print(json.dumps({'argv': sys.argv[1:], 'uid': os.getuid(), 'gid': os.getgid(),
                      'cwd': os.getcwd(), 'env': {name: os.environ.get(name) for name in (
                          'HOME', 'USER', 'LOGNAME', 'NETDATA_USER_CONFIG_DIR',
                          'NETDATA_STOCK_CONFIG_DIR', 'NETDATA_LIB_DIR', 'NETDATA_HOST_PREFIX')}}))
    sys.exit(int(os.environ.get('FIXTURE_STATUS', '0')))
''')
        self.adapter = self.root / 'adapter'
        self.adapter.write_text(ADAPTER.read_text()
                                .replace('/usr/sbin/netdata', str(self.agent))
                                .replace('/usr/libexec/netdata/plugins.d', str(self.root))
                                .replace('/var/lib/netdata/dem/runtime', str(self.runtime)))
        for path in (self.agent, self.plugin, self.adapter):
            path.chmod(0o755)
        self.credentials = ({'user': self.account.pw_uid, 'group': self.account.pw_gid, 'extra_groups': []}
                            if os.geteuid() == 0 else {})
        self.env = dict(os.environ, DOCKER_USR=self.account.pw_name, FIXTURE_CALLS=str(self.calls),
                        FIXTURE_PLAN=f'netdata-dem-runtime-setup-v1\nstandalone\nmanaged\n{self.runtime}',
                        NETDATA_HOST_PREFIX='/host')

    def run_adapter(self, *args, **env):
        return subprocess.run(['bash', str(self.adapter), *args], env=dict(self.env, **env),
                              **self.credentials, capture_output=True, text=True, timeout=10)

    def assert_call(self, result, action, flags=()):
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        report = json.loads(result.stdout)
        self.assertEqual(report['argv'], ['runtime', action, '--config-dir', str(self.root), *flags])
        self.assertEqual((report['uid'], report['gid']), (self.account.pw_uid, self.account.pw_gid))
        self.assertEqual(report['cwd'], '/')
        self.assertEqual(report['env'], {
            'HOME': str(self.root), 'USER': self.account.pw_name, 'LOGNAME': self.account.pw_name,
            'NETDATA_USER_CONFIG_DIR': str(self.root), 'NETDATA_STOCK_CONFIG_DIR': str(self.root),
            'NETDATA_LIB_DIR': str(self.root), 'NETDATA_HOST_PREFIX': '/host',
        })
        self.assertEqual(self.history.read_bytes(), b'preserve history\x00\xff')

    def test_pair_commands_forward_exact_service_environment_without_starting_agent(self):
        for action, flags in [('pair-plan', []), ('status', []), ('activate', []), ('rollback', []),
                              ('rollback', ['--prepared-only']),
                              ('fetch', ['--allow-download', '--output', str(self.root / 'inputs')]),
                              ('fetch', ['--release', '/inputs/release.json', '--archive', '/inputs/runtime.tar.zst',
                                         '--keyring', '/inputs/release-public.gpg',
                                         '--output', str(self.root / 'inputs')]),
                              ('prepare', ['--release', '/inputs/release.json', '--archive', '/inputs/runtime.tar.zst',
                                           '--keyring', '/inputs/release-public.gpg'])]:
            with self.subTest(action=action, flags=flags):
                self.assert_call(self.run_adapter(action, *flags), action, flags)
        calls = [json.loads(line) for line in self.calls.read_text().splitlines()]
        self.assertTrue(all(call[0] == 'runtime' for call in calls))
        self.assertFalse(any('qualify' in call or 'install' in call or 'ensure' in call for call in calls))

    def test_legacy_enrollment_protocol_is_unchanged(self):
        result = self.run_adapter('identity')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout, f'{self.account.pw_uid}:{self.account.pw_gid}\n')
        result = self.run_adapter('plan')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout, self.env['FIXTURE_PLAN'] + '\n')
        result = self.run_adapter('ensure')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads(result.stdout)['argv'],
                         ['runtime', 'ensure', '--allow-download', '--config-dir', str(self.root),
                          '--expected-root', str(self.runtime), '--cancel-on-stdin-close'])

    def test_prepare_never_downloads_and_fetch_requires_explicit_acquisition(self):
        for args in [('prepare', '--allow-download'), ('prepare',),
                     ('prepare', '--release', '/inputs/release.json'),
                     ('fetch', '--output', str(self.root / 'inputs')),
                     ('fetch', '--allow-download', '--archive', '/inputs/runtime.tar.zst',
                      '--output', str(self.root / 'inputs')),
                     ('fetch', '--allow-download', '--keyring', '/inputs/keyring',
                      '--output', str(self.root / 'inputs'))]:
            with self.subTest(args=args):
                result = self.run_adapter(*args)
                self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.calls.read_text(), '')

    def test_flags_cannot_replace_owner_config_or_runtime_root(self):
        for args in [('status', '--cache', '/elsewhere'), ('activate', '--allow-download'),
                     ('rollback', '--cache', '/elsewhere'), ('pair-plan', '--config-dir', '/elsewhere'),
                     ('fetch', '--root', '/elsewhere'), ('fetch', '--output=/inputs'),
                     ('fetch', '--allow-download', '--allow-download', '--output', '/inputs'),
                     ('fetch', '--output', '/inputs', '--output', '/other'),
                     ('fetch', '--output', 'relative'), ('fetch', '--output', '/control\nbytes'),
                     ('fetch', '--output'), ('plan', '--allow-download'), ('ensure', '--allow-download'),
                     ('identity', 'extra'), ('qualify',)]:
            with self.subTest(args=args):
                self.assertNotEqual(self.run_adapter(*args).returncode, 0)
        self.assertEqual(self.calls.read_text(), '')

    def test_fetch_cannot_retain_recovery_inputs_inside_managed_storage(self):
        alias = self.root / 'alias'
        alias.symlink_to(self.runtime, target_is_directory=True)
        existing = self.root / 'existing'
        existing.mkdir()
        for output in (self.runtime, self.runtime / 'inputs', alias / 'inputs',
                       self.root / 'runtime/../runtime/inputs', existing):
            with self.subTest(output=output):
                result = self.run_adapter('fetch', '--allow-download', '--output', str(output))
                self.assertNotEqual(result.returncode, 0)
                self.assertIn('outside the managed runtime root', result.stderr)
        calls = [json.loads(line) for line in self.calls.read_text().splitlines()]
        self.assertTrue(all(call[1] == 'setup-plan' for call in calls))

    def test_nonstandard_mode_or_root_does_not_change_storage(self):
        for plan in (f'netdata-dem-runtime-setup-v1\nstandalone\npreloaded\n{self.runtime}',
                     f'netdata-dem-runtime-setup-v1\nstandalone\nmanaged\n{self.root / "other"}',
                     'broken'):
            with self.subTest(plan=plan):
                result = self.run_adapter('activate', FIXTURE_PLAN=plan)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn('explicit deployment owner', result.stderr)
        calls = [json.loads(line) for line in self.calls.read_text().splitlines()]
        self.assertTrue(all(call[1] == 'setup-plan' for call in calls))

    def test_operation_status_and_safe_command_display(self):
        output = str(self.root / 'inputs with spaces;$(not-a-command)')
        flags = ['--allow-download', '--output', output]
        result = self.run_adapter('fetch', *flags)
        self.assert_call(result, 'fetch', flags)
        displayed = result.stderr.split('DEM browser runtime: run ', 1)[1].strip()
        self.assertEqual(shlex.split(displayed), [str(self.plugin), 'runtime', 'fetch',
                                                  '--config-dir', str(self.root), *flags])
        result = self.run_adapter('rollback', FIXTURE_STATUS='23')
        self.assertEqual(result.returncode, 23)
        self.assertIn('rollback failed in /: status 23', result.stderr)
        self.assertIn('paired deployment owner', result.stderr)
        displayed = next(line.split('run ', 1)[1] for line in result.stderr.splitlines()
                         if line.startswith('DEM browser runtime: run '))
        self.assertEqual(shlex.split(displayed), [str(self.plugin), 'runtime', 'rollback',
                                                  '--config-dir', str(self.root)])
        secret_output = str(self.root / 'secret=synthetic-credential')
        self.assert_call(self.run_adapter('fetch', '--allow-download', '--output', secret_output),
                         'fetch', ['--allow-download', '--output', secret_output])
        result = self.run_adapter('fetch', '--allow-download', '--output', secret_output)
        self.assertNotIn('synthetic-credential', result.stderr)
        displayed = result.stderr.split('DEM browser runtime: run ', 1)[1].strip()
        self.assertEqual(shlex.split(displayed)[-1], '[redacted]')


if __name__ == '__main__':
    unittest.main()

#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
"""Execute the host helper with isolated host-tool and service-identity fixtures.

These tests do not load host policy, switch real identities, or qualify browsers.
Fixed production paths are rewritten only in a temporary copy of the artifact.
"""
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import time
import unittest

HELPER = Path(__file__).resolve().parents[1] / 'netdata-dem-runtime.sh'
STUB = r'''#!/usr/bin/env python3
import fcntl
import hashlib
import json
import os
from pathlib import Path
import stat
import sys
root = Path(__FIXTURE__)
settings = json.loads((root / 'settings.json').read_text())
name = Path(sys.argv[0]).name
args = sys.argv[1:]
with (root / 'calls.jsonl').open('a') as log:
    log.write(json.dumps({'name': name, 'args': args, 'user': os.environ.get('FIXTURE_SERVICE_USER'),
                         'config': os.environ.get('NETDATA_USER_CONFIG_DIR'),
                         'lib': os.environ.get('NETDATA_LIB_DIR')}) + '\n')
if name == 'id':
    if len(args) > 1:
        if settings.get('uid_exit'): sys.exit(settings['uid_exit'])
        counter = root / 'uid-call-count'
        index = int(counter.read_text()) if counter.exists() else 0
        counter.write_text(str(index + 1))
        values = settings.get('uid_values', [settings.get('uid', '991')])
        print(values[min(index, len(values) - 1)])
    else:
        print(settings.get('root_uid', '0'))
elif name == 'stat':
    path = Path(args[-1])
    mode = (stat.S_IMODE(path.lstat().st_mode)
            if path == root or root in path.parents else 0o755)
    uid = '991' if str(path) in settings.get('untrusted', []) else '0'
    print(uid if args[1] == '%u' else uid + ':' + format(mode, 'o'))
elif name == 'readlink':
    value = str(Path(args[-1]).resolve()) if '-f' in args else os.readlink(args[-1])
    print(value, end='' if '-n' in args else '\n')
elif name == 'sha256sum':
    if settings.get('hash_exit'): sys.exit(settings['hash_exit'])
    print(hashlib.sha256(sys.stdin.buffer.read()).hexdigest() + '  -')
elif name == 'sync':
    assert args[0] == '-f', args
    sys.exit(settings.get('sync_exit', 0))
elif name == 'mv':
    values = [arg for arg in args if arg != '--']
    source, destination = map(Path, values)
    assert source.parent == destination.parent, (source, destination)
    if destination.parent == root / 'apparmor.d' and settings.get('publish_exit'):
        sys.exit(settings['publish_exit'])
    os.replace(source, destination)
elif name == 'flock':
    fcntl.flock(int(args[-1]), fcntl.LOCK_EX)
elif name == 'runuser':
    assert args[:3] == ['-u', settings['config']['global/run as user'], '--'], args
    os.environ['FIXTURE_SERVICE_USER'] = args[1]
    os.execvpe(args[3], args[3:], os.environ)
elif name == 'env':
    assert args.pop(0) == '-i'
    env = {}
    if 'FIXTURE_SERVICE_USER' in os.environ:
        env['FIXTURE_SERVICE_USER'] = os.environ['FIXTURE_SERVICE_USER']
    while args and '=' in args[0]:
        key, value = args.pop(0).split('=', 1)
        env[key] = value
    os.execvpe(args[0], args, env)
elif name == 'netdata':
    assert args[:2] == ['-W', 'get'], args
    if settings.get('root_query_context'):
        assert os.getcwd() == '/', os.getcwd()
        assert not os.environ.get('DEM_UNTRUSTED_ENV')
    if settings.get('query_exit'): sys.exit(settings['query_exit'])
    print(settings['config'][args[2] + '/' + args[3]])
elif name == 'dem.plugin':
    assert os.environ.get('FIXTURE_SERVICE_USER') == settings['config']['global/run as user']
    assert args[0] == 'runtime', args
    command = args[1]
    if command == 'setup-plan':
        if settings.get('plan_exit'): sys.exit(settings['plan_exit'])
        if 'plan' in settings:
            sys.stdout.buffer.write(settings['plan'].encode())
        else:
            mode = 'preloaded' if '--root' in args else settings.get('mode', 'managed')
            path = (args[args.index('--root') + 1] if '--root' in args else
                    args[args.index('--cache') + 1] if '--cache' in args else settings['runtime'])
            print('netdata-dem-runtime-setup-v1\n' + settings.get('delivery', 'standalone') + '\n' + mode + '\n' + path)
    elif command == 'uninstall':
        config_dir = Path(args[args.index('--config-dir') + 1])
        assert config_dir.stat().st_mode & 0o777 == 0o755
        assert (config_dir / 'dem.conf').read_text() == 'runtime:\n  mode: managed\n'
        assert (config_dir / 'dem.conf').stat().st_mode & 0o777 == 0o644
        if settings.get('drain_exit'): sys.exit(settings['drain_exit'])
        print('{"state":"uninstalled"}')
    else:
        if command == 'install' and settings.get('block_install'):
            import time
            (root / 'runtime-entered').write_text('entered')
            deadline = time.monotonic() + 15
            while not (root / 'runtime-release').exists():
                if time.monotonic() > deadline: raise SystemExit(99)
                time.sleep(0.02)
        if settings.get('cli_exit'): sys.exit(settings['cli_exit'])
        print('{"state":"installed"}')
elif name == 'apparmor_parser':
    source = Path(args[-1]).read_text()
    with (root / 'profiles-seen.jsonl').open('a') as log:
        log.write(json.dumps({'args': args, 'source': source}) + '\n')
    operation = 'parse' if '-Q' in args else 'remove' if '-R' in args else 'load'
    if settings.get(operation + '_exit'): sys.exit(settings[operation + '_exit'])
    profile = next(line.split()[1] for line in source.splitlines() if line.startswith('profile '))
    inventory = root / 'aa-profiles'
    lines = [line for line in inventory.read_text().splitlines() if not line.startswith(profile + ' (')]
    if operation == 'load' and not settings.get('load_without_inventory'):
        lines.append(profile + ' (unconfined)')
    if operation != 'parse': inventory.write_text(''.join(line + '\n' for line in lines))
else:
    raise SystemExit('unexpected fixture command: ' + name)
'''


class HostPolicyTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="dem-host-'test ")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name).resolve()
        self.bin = self.root / 'bin'
        self.bin.mkdir()
        self.aa = self.root / 'apparmor.d'
        self.aa.mkdir()
        (self.aa / 'disable').mkdir()
        (self.root / 'run').mkdir()
        (self.root / 'aa-enabled').write_text('Y\n')
        (self.root / 'aa-profiles').write_text('unrelated (enforce)\n')
        self.state = self.root / 'state'
        self.agent = self.bin / 'netdata'
        self.plugin = self.bin / 'dem.plugin'
        for name in ('id', 'stat', 'readlink', 'sha256sum', 'sync', 'mv', 'flock', 'runuser', 'env',
                     'netdata', 'dem.plugin', 'apparmor_parser'):
            target = self.bin / name
            target.write_text(STUB.replace('__FIXTURE__', repr(str(self.root))))
            target.chmod(0o755)
        self.settings = {
            'runtime': str(self.root / 'managed cache'),
            'config': {'global/run as user': 'monitor-user',
                       'directories/config': str(self.root / 'user config'),
                       'directories/stock config': str(self.root / 'stock config'),
                       'directories/lib': str(self.root / 'lib ; $(literal)'),
                       'directories/plugins': '"' + str(self.bin) + '"'},
        }
        source = HELPER.read_text()
        replacements = {
            'PATH=/usr/sbin:/usr/bin:/sbin:/bin': 'PATH=' + self.quote(str(self.bin) + ':' + os.environ['PATH']),
            'HOST_STATE=/var/lib/netdata-dem-runtime-host-policy': 'HOST_STATE=' + self.quote(str(self.state)),
            'HOST_RUN=/run': 'HOST_RUN=' + self.quote(str(self.root / 'run')),
            'HOST_APPARMOR_DIR=/etc/apparmor.d': 'HOST_APPARMOR_DIR=' + self.quote(str(self.aa)),
            'HOST_APPARMOR_ENABLED=/sys/module/apparmor/parameters/enabled':
                'HOST_APPARMOR_ENABLED=' + self.quote(str(self.root / 'aa-enabled')),
            'HOST_APPARMOR_PROFILES=/sys/kernel/security/apparmor/profiles':
                'HOST_APPARMOR_PROFILES=' + self.quote(str(self.root / 'aa-profiles')),
        }
        for before, after in replacements.items():
            self.assertIn(before, source)
            source = source.replace(before, after, 1)
        self.script = self.root / 'helper.sh'
        self.script.write_text(source)

    @staticmethod
    def quote(value):
        return "'" + value.replace("'", "'\\''") + "'"

    def invoke(self, *args):
        (self.root / 'settings.json').write_text(json.dumps(self.settings))
        return subprocess.run(['sh', str(self.script), *args], cwd=self.root, capture_output=True, text=True)

    def high(self, operation, *args):
        return self.invoke(operation, '--agent', str(self.agent), *args)

    def prepare(self, token='docker-one', path='/var/cache/netdata/dem/runtime'):
        return self.invoke('policy-prepare', '--deployment', token, '--runtime-root', path)

    def remove(self, token='docker-one'):
        return self.invoke('policy-remove', '--deployment', token, '--deployments-stopped')

    def calls(self, name=None):
        path = self.root / 'calls.jsonl'
        calls = [json.loads(line) for line in path.read_text().splitlines()] if path.exists() else []
        return [call for call in calls if name is None or call['name'] == name]

    def profiles(self):
        return list(self.aa.glob('netdata-dem-launcher-*'))

    def registrations(self):
        return list((self.state / 'registrations').glob('*'))

    def assertOK(self, result):
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_command_wrapper_preserves_status_escaping_and_secret_redaction(self):
        executable = self.root / "command ' literal"
        executable.write_text('#!/bin/sh\nprintf "%s\\n" "$1"\nexit "$2"\n')
        executable.chmod(0o755)
        # Source the actual artifact through its help entry point, then exercise
        # its command wrapper independently of runtime/policy semantics.
        body = 'script=$1; shift; set -- --help; . "$script" >/dev/null; host_run "$command" "$argument" "$status"'
        env = dict(os.environ, command=str(executable), argument='space ; $(literal) secret-token', status='0')
        result = subprocess.run(['sh', '-c', body, 'fixture', str(self.script)], env=env,
                                cwd=self.root, capture_output=True, text=True)
        self.assertOK(result)
        self.assertEqual(result.stdout, env['argument'] + '\n')
        self.assertNotIn('secret-token', result.stderr)
        self.assertIn(self.quote(str(executable)), result.stderr)
        env['status'] = '31'
        result = subprocess.run(['sh', '-c', body, 'fixture', str(self.script)], env=env,
                                cwd=self.root, capture_output=True, text=True)
        self.assertEqual(result.returncode, 31)
        self.assertIn('status 31', result.stderr)
        self.assertIn(str(self.root), result.stderr)

    def test_host_lock_covers_runtime_execution(self):
        self.settings['block_install'] = True
        (self.root / 'settings.json').write_text(json.dumps(self.settings))
        first = subprocess.Popen(['sh', str(self.script), 'install', '--agent', str(self.agent), '--allow-download'],
                                 cwd=self.root, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        second = None
        try:
            deadline = time.monotonic() + 15
            while not (self.root / 'runtime-entered').exists():
                if first.poll() is not None or time.monotonic() > deadline:
                    out, err = first.communicate(timeout=2)
                    self.fail('runtime did not enter blocking fixture: ' + err)
                time.sleep(0.02)
            second = subprocess.Popen(['sh', str(self.script), 'policy-prepare', '--deployment', 'concurrent',
                                       '--runtime-root', '/second/root'], cwd=self.root,
                                      stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
            time.sleep(0.25)
            self.assertIsNone(second.poll())
            self.assertEqual(len(self.registrations()), 1)
        finally:
            (self.root / 'runtime-release').write_text('release')
            out, err = first.communicate(timeout=20)
            self.assertEqual(first.returncode, 0, err)
            if second:
                out, err = second.communicate(timeout=20)
                self.assertEqual(second.returncode, 0, err)
        self.assertEqual(len(self.registrations()), 2)

    def test_policy_only_shared_setup_status_and_last_removal(self):
        self.assertOK(self.prepare())
        self.assertOK(self.prepare())
        self.assertOK(self.prepare('docker-two'))
        self.assertEqual(len(self.profiles()), 1)
        self.assertEqual(len(self.registrations()), 2)
        self.assertEqual(self.calls('netdata'), [])
        self.assertEqual(self.calls('dem.plugin'), [])
        result = self.invoke('policy-status', '--deployment', 'docker-one')
        self.assertOK(result)
        self.assertIn('loaded=yes', result.stdout)
        self.assertOK(self.remove())
        self.assertEqual(len(self.profiles()), 1)
        self.assertOK(self.remove('docker-two'))
        self.assertEqual(self.profiles(), [])
        self.assertEqual(self.registrations(), [])
        self.assertEqual((self.root / 'aa-profiles').read_text(), 'unrelated (enforce)\n')

    def test_docker_root_is_literal_execution_path_not_host_path(self):
        path = '/absent/space "quote" *?[ab]{x}@{HOME}^\\literal/exec'
        self.assertOK(self.prepare(path=path))
        source = self.profiles()[0].read_text()
        self.assertIn(r'\"quote\" \*\?\[ab\]\{x\}\@\{HOME\}\^\\literal', source)
        self.assertIn('/generations/*/chrome" flags=(unconfined)', source)
        self.assertEqual(source.count('flags=(unconfined)'), 1)

    def test_invalid_flags_and_paths_do_not_create_host_state(self):
        cases = [
            ('policy-prepare', '--deployment', '../escape', '--runtime-root', '/valid'),
            ('policy-prepare', '--deployment', 'one', '--runtime-root', '/bad\nline'),
            ('policy-prepare', '--deployment', 'one', '--runtime-root', '/bad/../root'),
            ('policy-prepare', '--deployment', 'one', '--runtime-root', '/bad//root'),
            ('policy-prepare', '--deployment', 'one', '--runtime-root', '/bad\troot'),
            ('policy-prepare', '--deployment', 'one', '--runtime-root', '/valid', '--allow-download'),
            ('policy-remove', '--deployment', 'one'),
            ('install', '--agent', str(self.agent)),
            ('install', '--agent', str(self.agent), '--allow-download', '--root', '/bad'),
            ('status', '--agent', str(self.agent), '--archive', '/bad'),
            ('agent-remove', '--agent', str(self.agent), '--cache', '/bad'),
            ('install', '--agent', str(self.agent), '--allow-download', '--unknown'),
        ]
        for args in cases:
            with self.subTest(args=args):
                self.assertNotEqual(self.invoke(*args).returncode, 0)
                self.assertFalse(self.state.exists())
                self.assertEqual(self.calls('apparmor_parser'), [])

    def test_absent_and_disabled_apparmor_skip_policy_but_retain_registration(self):
        (self.root / 'aa-enabled').unlink()
        self.assertOK(self.prepare())
        self.assertEqual(self.profiles(), [])
        self.assertEqual(self.calls('apparmor_parser'), [])
        self.assertEqual(len(self.registrations()), 1)
        self.assertOK(self.remove())
        (self.root / 'aa-enabled').write_text('N\n')
        self.assertOK(self.prepare())
        self.assertEqual(self.profiles(), [])
        self.assertOK(self.remove())

    def test_load_failure_is_retryable_and_status_reports_unloaded(self):
        self.settings['load_exit'] = 37
        self.assertEqual(self.prepare().returncode, 37)
        self.assertEqual(len(self.registrations()), 1)
        result = self.invoke('policy-status', '--deployment', 'docker-one')
        self.assertOK(result)
        self.assertIn('policy=yes\nloaded=no', result.stdout)
        self.settings.pop('load_exit')
        self.assertOK(self.prepare())
        self.assertOK(self.remove())

    def test_parser_validation_failure_keeps_retry_ownership(self):
        self.settings['parse_exit'] = 38
        self.assertEqual(self.prepare().returncode, 38)
        self.assertEqual(self.profiles(), [])
        self.assertEqual(len(self.registrations()), 1)
        self.settings.pop('parse_exit')
        self.assertOK(self.prepare())

    def test_failed_policy_publication_retains_reference_and_retries(self):
        self.settings['publish_exit'] = 46
        self.assertEqual(self.prepare().returncode, 46)
        self.assertEqual(self.profiles(), [])
        self.assertEqual(len(self.registrations()), 1)
        self.assertEqual(list(self.aa.glob('.netdata-dem-launcher-*')), [])
        self.settings.pop('publish_exit')
        self.assertOK(self.prepare())
        self.assertOK(self.remove())

    def test_digest_tool_failure_preserves_status_without_policy_mutation(self):
        self.settings['hash_exit'] = 44
        self.assertEqual(self.prepare().returncode, 44)
        self.assertEqual(self.profiles(), [])
        self.assertEqual(self.registrations(), [])
        self.assertEqual(self.calls('apparmor_parser'), [])

    def test_failed_sync_preserves_status_and_does_not_publish_policy(self):
        self.settings['sync_exit'] = 45
        self.assertEqual(self.prepare().returncode, 45)
        self.assertEqual(self.profiles(), [])
        self.assertEqual(self.registrations(), [])
        self.settings.pop('sync_exit')
        self.assertOK(self.prepare())

    def test_parser_success_without_loaded_policy_is_failure(self):
        self.settings['load_without_inventory'] = True
        self.assertNotEqual(self.prepare().returncode, 0)
        self.assertEqual(len(self.registrations()), 1)

    def test_unload_failure_preserves_policy_and_reference_for_retry(self):
        self.assertOK(self.prepare())
        self.settings['remove_exit'] = 39
        self.assertEqual(self.remove().returncode, 39)
        self.assertEqual(len(self.profiles()), 1)
        self.assertEqual(len(self.registrations()), 1)
        self.settings.pop('remove_exit')
        self.assertOK(self.remove())

    def test_admin_edit_disable_and_symlink_are_authoritative(self):
        self.assertOK(self.prepare())
        profile = self.profiles()[0]
        original = profile.read_text()
        profile.write_text(original + '# administrator edit\n')
        for result in (self.prepare(), self.remove()):
            self.assertNotEqual(result.returncode, 0)
        self.assertEqual(len(self.registrations()), 1)
        profile.write_text(original)
        disabled = self.aa / 'disable' / profile.name
        disabled.symlink_to(profile)
        self.assertNotEqual(self.prepare().returncode, 0)
        self.assertNotEqual(self.remove().returncode, 0)
        disabled.unlink()
        profile.unlink()
        profile.symlink_to(self.root / 'aa-profiles')
        self.assertNotEqual(self.prepare().returncode, 0)

    def test_unexpected_loaded_mode_is_preserved(self):
        self.assertOK(self.prepare())
        inventory = self.root / 'aa-profiles'
        original = inventory.read_text()
        inventory.write_text(original.replace('(unconfined)', '(enforce)'))
        self.assertNotEqual(self.prepare().returncode, 0)
        self.assertNotEqual(self.remove().returncode, 0)
        self.assertEqual(len(self.registrations()), 1)
        self.assertIn('(enforce)', inventory.read_text())

    def test_missing_persistent_live_policy_refuses_then_safe_absent_retry(self):
        self.assertOK(self.prepare())
        self.profiles()[0].unlink()
        self.assertNotEqual(self.remove().returncode, 0)
        self.assertEqual(len(self.registrations()), 1)
        (self.root / 'aa-profiles').write_text('unrelated (enforce)\n')
        self.assertOK(self.remove())

    def test_unknown_and_malformed_metadata_never_loses_shared_policy(self):
        self.assertOK(self.prepare())
        record = self.registrations()[0]
        original = record.read_text()
        for bad in ('future-version\n', original + 'extra\n', original.replace('external', 'unknown'),
                    original.replace('/var/cache', '/var/\x00cache')):
            record.write_text(bad)
            self.assertNotEqual(self.remove().returncode, 0)
            self.assertEqual(len(self.profiles()), 1)
        record.write_text(original)
        self.assertOK(self.remove())

    def test_root_and_state_ownership_fail_closed(self):
        self.settings['root_uid'] = '991'
        self.assertNotEqual(self.prepare().returncode, 0)
        self.assertFalse(self.state.exists())
        self.settings.pop('root_uid')
        self.state.mkdir(mode=0o777)
        self.state.chmod(0o777)
        self.assertNotEqual(self.prepare().returncode, 0)
        self.state.chmod(0o700)
        (self.state / 'lock').symlink_to(self.root / 'aa-profiles')
        self.assertNotEqual(self.prepare().returncode, 0)

    def test_high_level_setup_uses_service_identity_and_preserves_arguments(self):
        release = str(self.root / 'release $(literal).json')
        archive = str(self.root / "archive ' literal.tar.zst")
        result = self.high('install', '--release', release, '--archive', archive, '--manual-updates')
        self.assertOK(result)
        calls = self.calls('dem.plugin')
        self.assertEqual([call['args'][1] for call in calls], ['setup-plan', 'install'])
        install = calls[-1]
        self.assertEqual(install['user'], 'monitor-user')
        self.assertIn(release, install['args'])
        self.assertIn(archive, install['args'])
        self.assertEqual(install['lib'], self.settings['config']['directories/lib'])
        self.assertNotIn(release, result.stderr)
        self.assertNotIn(archive, result.stderr)
        self.assertIn('arguments redacted', result.stderr)
        self.assertEqual(len(self.registrations()), 1)

    def test_qualified_service_identity_round_trip(self):
        for user in ('monitor@example.test', r'EXAMPLE\monitor', '-monitor', '--help'):
            with self.subTest(user=user):
                self.settings['config']['global/run as user'] = user
                self.assertOK(self.high('install', '--allow-download'))
                record = self.registrations()[0]
                self.assertEqual(record.read_text().splitlines()[4], user)
                self.assertTrue(any(c['args'] == ['-u', '--', user] for c in self.calls('id')))
                self.assertOK(self.high('status'))
                Path(self.settings['runtime']).mkdir(exist_ok=True)
                self.assertOK(self.high('uninstall'))
                self.assertEqual(self.registrations(), [])
                self.assertEqual(self.profiles(), [])
                self.assertTrue(any(c['args'][1] == 'uninstall' and c['user'] == user
                                    for c in self.calls('dem.plugin')))

    def test_service_identity_rejects_invalid_text_before_lookup(self):
        for user in ('', 'bad\nline', 'bad\n', 'bad\n\n', 'bad\tname', 'bad\x01name',
                     'bad\x7fname', 'x' * 4097):
            with self.subTest(user=user):
                self.settings['config']['global/run as user'] = user
                self.assertNotEqual(self.high('install', '--allow-download').returncode, 0)
                self.assertEqual([c for c in self.calls('id') if len(c['args']) > 1], [])
                self.assertEqual(self.calls('runuser'), [])
                self.assertEqual(self.registrations(), [])

    def test_service_identity_query_failure_preserves_original_status(self):
        self.settings['query_exit'] = 37
        self.assertEqual(self.high('install', '--allow-download').returncode, 37)
        self.assertEqual([c for c in self.calls('id') if len(c['args']) > 1], [])
        self.assertEqual(self.calls('runuser'), [])
        self.assertEqual(self.registrations(), [])

    def test_registration_identity_is_validated_without_live_nss_lookup(self):
        self.settings['config']['global/run as user'] = 'monitor@example.test'
        self.assertOK(self.high('install', '--allow-download'))
        self.settings['uid_exit'] = 19
        before = len(self.calls('id'))
        self.assertOK(self.prepare('unrelated'))
        self.assertOK(self.remove('unrelated'))
        self.assertEqual(len(self.calls('id')), before + 2)  # Administrator checks only.
        self.assertEqual(self.high('status').returncode, 19)
        self.assertEqual(len(self.registrations()), 1)

    def test_invalid_stored_service_identity_blocks_inventory(self):
        self.assertOK(self.high('install', '--allow-download'))
        record = self.registrations()[0]
        original = record.read_text().splitlines()
        for user in ('', 'bad\tname', 'bad\x7fname', 'x' * 4097):
            with self.subTest(user=user):
                fields = original.copy()
                fields[4] = user
                record.write_text('\n'.join(fields) + '\n')
                self.assertNotEqual(self.prepare('unrelated').returncode, 0)
                self.assertEqual(len(self.registrations()), 1)

    def test_uid_zero_alias_is_rejected_at_discovery_and_execution(self):
        self.settings['config']['global/run as user'] = 'administrator@example.test'
        self.settings['uid'] = '0'
        self.assertNotEqual(self.high('install', '--allow-download').returncode, 0)
        self.assertEqual(self.calls('runuser'), [])
        self.assertEqual(self.registrations(), [])
        self.settings['uid_values'] = ['991', '991', '0']
        (self.root / 'uid-call-count').unlink(missing_ok=True)
        result = self.high('install', '--allow-download')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('no longer nonroot', result.stderr)
        self.assertEqual([c['args'][1] for c in self.calls('dem.plugin')], ['setup-plan'])
        self.assertEqual(len(self.registrations()), 1)

    def test_runtime_failure_retains_ready_policy_and_original_status(self):
        self.settings['cli_exit'] = 41
        result = self.high('repair', '--allow-download')
        self.assertEqual(result.returncode, 41)
        self.assertEqual(len(self.registrations()), 1)
        self.assertEqual(len(self.profiles()), 1)
        self.assertIn('status 41', result.stderr)

    def test_rejects_untrusted_agent_or_plugin_and_root_service(self):
        for path in (self.agent, self.plugin):
            self.settings['untrusted'] = [str(path)]
            self.assertNotEqual(self.high('install', '--allow-download').returncode, 0)
            self.assertEqual(self.calls('apparmor_parser'), [])
        self.settings.pop('untrusted')
        self.settings['uid'] = '0'
        self.assertNotEqual(self.high('install', '--allow-download').returncode, 0)
        self.assertEqual(self.calls('dem.plugin'), [])

    def test_rejected_path_reports_permissions_without_changing_them(self):
        cases = {
            'group-writable Agent': (self.agent, 0o775, '0'),
            'group-writable plugin': (self.plugin, 0o775, '0'),
            'group-writable parent': (self.bin, 0o775, '0'),
            'world-writable parent': (self.bin, 0o757, '0'),
            'nonroot Agent': (self.agent, 0o755, '991'),
            'nonroot plugin': (self.plugin, 0o755, '991'),
            'nonroot parent': (self.bin, 0o755, '991'),
        }
        for name, (path, mode, uid) in cases.items():
            with self.subTest(name=name):
                original_mode = path.stat().st_mode & 0o7777
                path.chmod(mode)
                self.settings['untrusted'] = [] if uid == '0' else [str(path)]
                agent_calls = len(self.calls('netdata'))
                try:
                    result = self.high('install', '--allow-download')
                    self.assertEqual(result.returncode, 1)
                    self.assertEqual(result.stdout, '')
                    self.assertIn(str(path), result.stderr)
                    self.assertIn('owner UID=' + uid, result.stderr)
                    self.assertIn('mode=' + format(mode, '04o'), result.stderr)
                    self.assertIn('namei -l', result.stderr)
                    self.assertIn('chmod go-w', result.stderr)
                    self.assertEqual(path.stat().st_mode & 0o7777, mode)
                    if path != self.plugin:
                        self.assertEqual(len(self.calls('netdata')), agent_calls)
                    self.assertEqual(self.calls('dem.plugin'), [])
                    self.assertEqual(self.calls('apparmor_parser'), [])
                    self.assertEqual(self.registrations(), [])
                    self.assertEqual(self.profiles(), [])
                finally:
                    path.chmod(original_mode)
                    self.settings.pop('untrusted')

    def test_untrusted_alias_reports_link_ownership(self):
        alias = self.root / 'agent alias'
        alias.symlink_to(self.agent)
        self.settings['untrusted'] = [str(alias)]
        result = self.invoke('install', '--agent', str(alias), '--allow-download')
        self.assertEqual(result.returncode, 1)
        self.assertEqual(result.stdout, '')
        self.assertIn(str(alias), result.stderr)
        self.assertIn('owner UID=991', result.stderr)
        self.assertIn('mode=' + format(alias.lstat().st_mode & 0o7777, '04o'), result.stderr)
        self.assertIn('symbolic link', result.stderr)
        self.assertIn('namei -l', result.stderr)
        self.assertNotIn('chmod go-w', result.stderr)
        self.assertTrue(alias.is_symlink())
        self.assertEqual(self.calls('netdata'), [])
        self.assertEqual(self.calls('dem.plugin'), [])
        self.assertEqual(self.registrations(), [])
        self.assertEqual(self.profiles(), [])

    def test_alias_target_reports_writable_ancestor_before_execution(self):
        alias = self.root / 'agent alias'
        alias.symlink_to(self.agent)
        self.bin.chmod(0o775)
        result = self.invoke('install', '--agent', str(alias), '--allow-download')
        self.assertEqual(result.returncode, 1)
        self.assertIn(str(self.bin), result.stderr)
        self.assertIn('owner UID=0', result.stderr)
        self.assertIn('mode=0775', result.stderr)
        self.assertEqual(self.bin.stat().st_mode & 0o7777, 0o775)
        self.assertEqual(self.calls('netdata'), [])
        self.assertEqual(self.calls('dem.plugin'), [])
        self.assertEqual(self.registrations(), [])
        self.assertEqual(self.profiles(), [])

    def test_setup_protocol_invalid_output_never_registers_policy(self):
        valid = 'netdata-dem-runtime-setup-v1\nstandalone\nmanaged\n/valid\n'
        for output in (valid + '\n', valid.replace('v1', 'v2'), valid.replace('managed', 'unexpected'),
                       valid.replace('standalone', 'unexpected'),
                       valid.replace('/valid', '/bad\x00path'), valid.replace('/valid', '/bad\tpath'),
                       valid.replace('/valid', '/bad\npath'), valid.replace('/valid', '/' + 'x' * 4097)):
            self.settings['plan'] = output
            result = self.high('install', '--allow-download')
            self.assertNotEqual(result.returncode, 0, output)
            self.assertEqual(self.registrations(), [])
            self.assertEqual(self.calls('apparmor_parser'), [])

    def test_configured_preload_refuses_managed_install_without_registration(self):
        self.settings['mode'] = 'preloaded'
        self.assertNotEqual(self.high('install', '--allow-download').returncode, 0)
        self.assertEqual(self.registrations(), [])
        self.assertEqual(self.calls('apparmor_parser'), [])

    def test_native_mode_refuses_managed_mutations(self):
        self.settings['mode'] = 'native'
        self.assertNotEqual(self.high('install', '--allow-download').returncode, 0)
        self.assertEqual(self.registrations(), [])
        self.assertOK(self.high('agent-remove'))
        self.assertEqual([c['args'][1] for c in self.calls('dem.plugin')], ['setup-plan', 'setup-plan'])

    def test_native_packaged_agent_cannot_create_managed_or_preload_policy(self):
        self.settings['delivery'] = 'native-package'
        self.settings['mode'] = 'managed'
        for action, flags in [('install', []), ('repair', []), ('preload', ['--root', str(self.root / 'preload')])]:
            result = self.high(action, '--allow-download', *flags)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn('optional netdata-dem-runtime package', result.stderr)
            self.assertEqual(self.registrations(), [])
            self.assertEqual(self.calls('apparmor_parser'), [])
            self.assertTrue(all(c['args'][1] == 'setup-plan' for c in self.calls('dem.plugin')))

    def test_uninstall_drain_failure_retains_policy_then_retry_succeeds(self):
        self.assertOK(self.high('install', '--allow-download'))
        Path(self.settings['runtime']).mkdir()
        self.settings['drain_exit'] = 43
        self.assertEqual(self.high('uninstall').returncode, 43)
        self.assertEqual(len(self.registrations()), 1)
        self.assertEqual(len(self.profiles()), 1)
        self.assertEqual(list((self.root / 'run').iterdir()), [])
        self.settings.pop('drain_exit')
        self.assertOK(self.high('uninstall'))
        self.assertEqual(self.registrations(), [])
        self.assertEqual(self.profiles(), [])

    def test_agent_remove_drains_retained_roots_after_mode_and_config_change(self):
        self.assertOK(self.high('install', '--allow-download'))
        previous = self.settings['runtime']
        Path(previous).mkdir()
        self.settings['runtime'] = str(self.root / 'new absent cache')
        self.settings['mode'] = 'native'
        self.assertOK(self.high('agent-remove'))
        drains = [c for c in self.calls('dem.plugin') if c['args'][1] == 'uninstall']
        self.assertEqual(len(drains), 1)
        self.assertIn(previous, drains[0]['args'])
        self.assertEqual(self.registrations(), [])
        self.assertFalse(Path(self.settings['runtime']).exists())

    def test_agent_remove_drains_known_root_then_refuses_damaged_current_config(self):
        self.assertOK(self.high('install', '--allow-download'))
        Path(self.settings['runtime']).mkdir()
        self.settings['plan_exit'] = 47
        self.assertEqual(self.high('agent-remove').returncode, 47)
        self.assertEqual(self.registrations(), [])
        self.assertEqual(len([c for c in self.calls('dem.plugin') if c['args'][1] == 'uninstall']), 1)

    def test_agent_remove_unregistered_present_cache_and_absent_cache(self):
        self.assertOK(self.high('agent-remove'))
        self.assertEqual([c['args'][1] for c in self.calls('dem.plugin')], ['setup-plan'])
        Path(self.settings['runtime']).mkdir()
        self.assertOK(self.high('agent-remove'))
        self.assertEqual([c['args'][1] for c in self.calls('dem.plugin')][-2:], ['setup-plan', 'uninstall'])

    def test_no_policy_removal_does_not_require_apparmor_access(self):
        (self.root / 'aa-profiles').unlink()
        (self.bin / 'apparmor_parser').unlink()
        self.assertOK(self.high('agent-remove'))
        Path(self.settings['runtime']).mkdir()
        self.assertOK(self.high('agent-remove'))
        self.assertEqual([c['args'][1] for c in self.calls('dem.plugin')],
                         ['setup-plan', 'setup-plan', 'uninstall'])
        self.assertEqual(self.calls('apparmor_parser'), [])
        self.assertNotEqual(self.high('install', '--allow-download').returncode, 0)

    def test_agent_symlink_cannot_redirect_privileged_queries_to_interpreter(self):
        self.agent.unlink()
        self.agent.symlink_to(Path(sys.executable).resolve())
        self.settings['untrusted'] = [str(self.bin)]
        marker = self.root / 'executed'
        (self.root / 'global').write_text(
            'from pathlib import Path\nPath(' + repr(str(marker)) + ').touch()\nprint("fixture-user")\n')
        self.assertNotEqual(self.high('agent-remove').returncode, 0)
        self.assertFalse(marker.exists(), 'service-owned cwd script was executed through Agent symlink')

    def test_trusted_static_aliases_resolve_without_losing_chain_ownership(self):
        usr = self.root / 'usr'
        usr.mkdir()
        (usr / 'sbin').symlink_to('../bin')
        (usr / 'libexec').symlink_to('../bin')
        self.settings['config']['directories/plugins'] = '"' + str(usr / 'libexec') + '"'
        self.assertOK(self.invoke('install', '--agent', str(usr / 'sbin/netdata'), '--allow-download'))
        for path in (usr, self.bin):
            self.settings['untrusted'] = [str(path)]
            self.assertNotEqual(self.invoke('agent-remove', '--agent', str(usr / 'sbin/netdata')).returncode, 0)

    def test_agent_alias_cannot_cross_untrusted_target_or_loop(self):
        aliases = self.root / 'aliases'
        aliases.mkdir()
        (aliases / 'agent').symlink_to('../bin/netdata')
        self.settings['untrusted'] = [str(self.bin)]
        self.assertNotEqual(self.invoke('agent-remove', '--agent', str(aliases / 'agent')).returncode, 0)
        self.assertEqual(self.calls('netdata'), [])
        self.settings.pop('untrusted')
        (aliases / 'a').symlink_to('b')
        (aliases / 'b').symlink_to('a')
        self.assertNotEqual(self.invoke('agent-remove', '--agent', str(aliases / 'a')).returncode, 0)
        self.assertEqual(self.calls('netdata'), [])
        (aliases / 'control').symlink_to('../bin/netdata\n')
        self.assertNotEqual(self.invoke('agent-remove', '--agent', str(aliases / 'control')).returncode, 0)
        self.assertEqual(self.calls('netdata'), [])

    def test_privileged_queries_use_fixed_cwd_and_clean_environment(self):
        self.settings['root_query_context'] = True
        previous = os.environ.get('DEM_UNTRUSTED_ENV')
        os.environ['DEM_UNTRUSTED_ENV'] = 'service-owned'
        try:
            self.assertOK(self.high('agent-remove'))
        finally:
            if previous is None:
                os.environ.pop('DEM_UNTRUSTED_ENV', None)
            else:
                os.environ['DEM_UNTRUSTED_ENV'] = previous

    def test_agent_remove_drains_matching_current_registration_once(self):
        self.assertOK(self.high('install', '--allow-download'))
        Path(self.settings['runtime']).mkdir()
        self.assertOK(self.high('agent-remove'))
        self.assertEqual(len([c for c in self.calls('dem.plugin') if c['args'][1] == 'uninstall']), 1)
        self.assertEqual(self.registrations(), [])

    def test_configured_unregistered_preload_requires_explicit_retirement(self):
        self.settings['mode'] = 'preloaded'
        self.assertNotEqual(self.high('agent-remove').returncode, 0)
        self.assertOK(self.high('agent-remove', '--deployments-stopped'))
        self.assertEqual([c for c in self.calls('dem.plugin') if c['args'][1] == 'uninstall'], [])

    def test_targeted_preload_cleanup_preserves_managed_registration(self):
        self.assertOK(self.high('install', '--allow-download'))
        preload = self.root / 'separate preload'
        self.assertOK(self.high('preload', '--root', str(preload), '--allow-download'))
        preload.mkdir()
        self.assertNotEqual(self.high('uninstall', '--root', str(preload)).returncode, 0)
        self.assertOK(self.high('uninstall', '--root', str(preload), '--deployments-stopped'))
        self.assertEqual(len(self.registrations()), 1)
        self.assertEqual(len(self.profiles()), 1)
        self.assertTrue(preload.is_dir())
        self.assertEqual([c for c in self.calls('dem.plugin') if c['args'][1] == 'uninstall'], [])

    def test_preload_requires_explicit_retirement_and_keeps_payload(self):
        preload = self.root / 'protected preload'
        self.assertOK(self.high('preload', '--root', str(preload), '--allow-download'))
        preload.mkdir()
        self.assertNotEqual(self.high('agent-remove').returncode, 0)
        self.assertEqual(len(self.registrations()), 1)
        self.assertOK(self.high('agent-remove', '--deployments-stopped'))
        self.assertTrue(preload.is_dir())
        self.assertEqual(self.registrations(), [])
        self.assertEqual([c for c in self.calls('dem.plugin') if c['args'][1] == 'uninstall'], [])


if __name__ == '__main__':
    unittest.main()

#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
"""Exercise standalone kickstart dispatch and Docker host trust-boundary fixtures.

The Docker fixture cannot establish real sandbox/locality qualification. All fixed
host paths and host-tool metadata are redirected only in its temporary artifact.
"""
import hashlib
import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import tempfile
import time
import unittest

INSTALLER = Path(__file__).resolve().parents[1]
CONTAINER_ID = 'a' * 64
STUB = r'''#!/usr/bin/env python3
import fcntl, json, os, shutil, signal, stat, sys, time
from pathlib import Path
root = Path(__ROOT__)
settings = json.loads((root / 'settings').read_text())
name = Path(sys.argv[0]).name
args = sys.argv[1:]
with (root / 'calls').open('a') as f:
    f.write(json.dumps([name, args]) + '\n')
if name == 'id': print('0')
elif name == 'uname': print('Linux')
elif name == 'stat':
    target = Path(args[-1])
    mode = stat.S_IMODE(target.stat().st_mode) if root in target.parents or target == root else 0o755
    kind = 'symbolic link' if target.is_symlink() else 'directory' if target.is_dir() else 'regular file'
    if args[1] == '%F:%u:%a': print(f'{kind}:0:{mode:o}')
    elif args[1] == '%F:%u': print(f'{kind}:0')
    elif args[1] == '%u': print('0')
    else: print('0:' + format(mode, 'o'))
elif name == 'findmnt': print(settings.get('fstype', 'ext4'))
elif name == 'apparmor_parser':
    if settings.get('policy_exit'): sys.exit(settings['policy_exit'])
    text = Path(args[-1]).read_text()
    profile = next(line.split()[1] for line in text.splitlines() if line.startswith('profile '))
    if '-Q' not in args:
        (root / 'profiles').write_text('' if '-R' in args else profile + ' (unconfined)\n')
elif name == 'sync': pass
elif name == 'flock': fcntl.flock(int(args[-1]), fcntl.LOCK_EX)
elif name == 'docker':
    if settings.get('diagnostic'): sys.stderr.buffer.write(settings['diagnostic'].encode('latin1'))
    assert args[:3] == ['--host', 'unix:///var/run/docker.sock', '--config'], args
    assert 'DOCKER_HOST' not in os.environ
    args = args[4:]
    if settings.get('no_daemon'): raise AssertionError('Docker used after deployment retirement')
    if args[0] == 'create':
        assert args[:-2] == ['create', '--pull=never', '--network=none', '--cidfile'], args
        assert args[-1] == 'sha256:' + 'c' * 64
        Path(args[-2]).write_text('d' * 64)
        (root / 'scratch').touch()
        if settings.get('create_exit'): sys.exit(settings['create_exit'])
        print('d' * 64)
    elif args[0] == 'cp':
        assert (root / 'scratch').exists() and args[1].startswith('d' * 64 + ':/usr/libexec/netdata/'), args
        if settings.get('copy_exit'): sys.exit(settings['copy_exit'])
        filename = Path(args[1]).name
        if settings.get('link'): Path(args[2]).symlink_to(root / 'helper')
        else: shutil.copyfile(root / filename, args[2])
    elif args[0] == 'rm':
        assert args == ['rm', '-v', 'd' * 64]
        (root / 'scratch').unlink()
    elif args[0] == 'info':
        template = args[2]
        if template == '{{.ID}}': print('trusted-daemon')
        elif template == '{{.DockerRootDir}}': print(root / 'data')
        else: print(settings.get('security', '["name=apparmor"]|Ubuntu 24.04'))
    elif args[:2] == ['volume', 'inspect']:
        print(settings.get('volume', 'local|local|null'))
    elif args[0] == 'inspect':
        template = args[4]
        if template == '{{.Id}}':
            if args[-1] == 'monitor' and (settings.get('replaced') or
                    settings.get('swapped') and (root / 'ensured').exists()): print('b' * 64)
            else: print('a' * 64)
        elif '{{.Image}}' in template:
            print('a' * 64 + '|sha256:' + 'c' * 64 + '|/monitor|true|123')
        elif template.startswith('{{.Id}}|'):
            print('a' * 64 + '|/monitor|true|123|2026-11-04T00:00:00Z')
        elif template.startswith('{{json .Config.Entrypoint}}'):
            print(settings.get('invocation', '["/usr/sbin/run.sh"]|null|/usr/sbin/run.sh|[]||unconfined|false|'))
        else:
            mounts = settings.get('mounts', [
                ['volume', str(root / 'data/volumes/store/_data'), '/var/lib/netdata', True, 'store']])
            for kind, source, dest, rw, volume in mounts:
                print('|'.join([json.dumps(kind), json.dumps(source), json.dumps(dest),
                                str(rw).lower(), json.dumps(volume)]))
    elif args[0] == 'exec':
        operation = args[args.index('/usr/libexec/netdata/netdata-dem-docker') + 1]
        user = args[args.index('--user') + 1]
        if operation == 'identity':
            assert user == '65534:65534', args
            print(settings.get('identity', '201:202'))
        elif operation == 'plan':
            assert user == settings.get('identity', '201:202'), args
            print(settings.get('plan',
                               'netdata-dem-runtime-setup-v1\nstandalone\nmanaged\n/var/lib/netdata/dem/runtime'))
        elif operation == 'ensure':
            assert user == '201:202' and args[-1] == 'ensure', args
            (root / 'ensured').write_text('yes')
            if settings.get('block'):
                signal.alarm(8)
                sys.stdin.buffer.read()
                (root / 'cancelled').write_text('yes')
                time.sleep(0.15)
                sys.exit(23)
            if settings.get('ensure_output'): sys.stdout.write('x' * settings['ensure_output'])
            if settings.get('ensure_text'): sys.stdout.buffer.write(settings['ensure_text'].encode('latin1'))
            sys.exit(settings.get('ensure_exit', 0))
        else: raise AssertionError(args)
    else: raise AssertionError(args)
else: raise AssertionError(name)
'''


class DockerHostTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='dem-docker-host-')
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name).resolve()
        self.bin = self.root / 'bin'
        self.bin.mkdir()
        # The helper fixes PATH; retain the interpreter running this fixture.
        (self.bin / 'python3').symlink_to(sys.executable)
        for path in ('run', 'data/volumes/store/_data', 'proc/123', 'apparmor.d/disable'):
            (self.root / path).mkdir(parents=True)
        (self.root / 'var-run').symlink_to(self.root / 'run')
        (self.root / 'run/docker.sock').touch()
        (self.root / 'proc/123/cgroup').write_text('0::/system.slice/docker-' + CONTAINER_ID + '.scope\n')
        (self.root / 'proc/123/uid_map').write_text('0 0 4294967295\n')
        (self.root / 'context').write_text('unconfined\n')
        (self.root / 'aa').write_text('Y\n')
        (self.root / 'profiles').write_text('')
        (self.root / 'restrict').write_text('1\n')
        (self.root / 'unconfined').write_text('0\n')
        (self.root / 'clone').write_text('1\n')
        (self.root / 'os-release').write_text('ID=ubuntu\nVERSION_ID="24.04"\n')
        for name in ('id', 'uname', 'stat', 'findmnt', 'docker', 'apparmor_parser', 'sync', 'flock'):
            p = self.bin / name
            p.write_text(STUB.replace('__ROOT__', repr(str(self.root))))
            p.chmod(0o755)
        sources = {path.name: path.read_text() for path in
                   [INSTALLER / 'netdata-dem-runtime.sh', INSTALLER.parent / 'docker/netdata-dem-docker-host.sh']}
        replacements = {
            'PATH=/usr/sbin:/usr/bin:/sbin:/bin': 'PATH=' + str(self.bin) + ':/usr/sbin:/usr/bin:/sbin:/bin',
            'HOST_STATE=/var/lib/netdata-dem-runtime-host-policy': 'HOST_STATE=' + str(self.root / 'state'),
            'HOST_RUN=/run': 'HOST_RUN=' + str(self.root / 'run'),
            'HOST_APPARMOR_DIR=/etc/apparmor.d': 'HOST_APPARMOR_DIR=' + str(self.root / 'apparmor.d'),
            'HOST_APPARMOR_ENABLED=/sys/module/apparmor/parameters/enabled':
                'HOST_APPARMOR_ENABLED=' + str(self.root / 'aa'),
            'HOST_APPARMOR_PROFILES=/sys/kernel/security/apparmor/profiles':
                'HOST_APPARMOR_PROFILES=' + str(self.root / 'profiles'),
            '/.dockerenv': str(self.root / 'nested'),
            '/run/.containerenv': str(self.root / 'containerenv'),
            '/proc/self/attr/current': str(self.root / 'context'),
            'host_ancestors /run &&': 'host_ancestors ' + str(self.root / 'run') + ' &&',
            'readlink -f -- /var/run': 'readlink -f -- ' + str(self.root / 'var-run'),
            '" = /run ]': '" = ' + str(self.root / 'run') + ' ]',
            'stat -c %u /var/run': 'stat -c %u ' + str(self.root / 'var-run'),
            '[ -S /var/run/docker.sock ]': '[ -f ' + str(self.root / 'run/docker.sock') + ' ]',
            '[ ! -L /var/run/docker.sock ]': '[ ! -L ' + str(self.root / 'run/docker.sock') + ' ]',
            'stat -c %u /var/run/docker.sock': 'stat -c %u ' + str(self.root / 'run/docker.sock'),
            'host_resolve_executable /usr/bin/docker': 'host_resolve_executable ' + str(self.bin / 'docker'),
            'mktemp -d /run/netdata-dem-docker.XXXXXXXX':
                'mktemp -d ' + str(self.root / 'run/netdata-dem-docker.XXXXXXXX'),
            '/proc/$docker_pid': str(self.root / 'proc/$docker_pid'),
            '/proc/sys/kernel/apparmor_restrict_unprivileged_userns': str(self.root / 'restrict'),
            '/proc/sys/kernel/apparmor_restrict_unprivileged_unconfined': str(self.root / 'unconfined'),
            '/proc/sys/kernel/unprivileged_userns_clone': str(self.root / 'clone'),
            '/etc/os-release': str(self.root / 'os-release'),
        }
        # Longest first prevents the socket path sharing its /var/run prefix.
        for before, after in sorted(replacements.items(), key=lambda item: -len(item[0])):
            self.assertTrue(any(before in source for source in sources.values()), before)
            sources = {name: source.replace(before, after) for name, source in sources.items()}
        for name, source in sources.items():
            (self.root / name).write_text(source)
        self.script = self.root / 'netdata-dem-runtime.sh'
        self.settings = {}
        self.token = 'docker-' + hashlib.sha256(
            b'netdata-dem-docker-registration-v1:trusted-daemon:monitor').hexdigest()

    def invoke(self, *args):
        (self.root / 'settings').write_text(json.dumps(self.settings))
        args = args or ('docker-install', '--container', 'monitor')
        if args[0] == 'docker-install':
            args = (*args, '--container-id', CONTAINER_ID)
        return subprocess.run(['sh', str(self.script), *args],
                              capture_output=True, text=True, timeout=20)

    def calls(self):
        p = self.root / 'calls'
        return [json.loads(line) for line in p.read_text().splitlines()] if p.exists() else []

    def assertOK(self, result):
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_dry_run_inspects_but_never_registers_or_ensures(self):
        result = self.invoke('docker-install', '--container', 'monitor', '--dry-run')
        self.assertOK(result)
        self.assertIn('host-policy=needed', result.stderr)
        self.assertFalse((self.root / 'state').exists())
        self.assertFalse((self.root / 'ensured').exists())

    def test_registration_retry_and_retirement_without_container(self):
        self.assertOK(self.invoke())
        self.assertOK(self.invoke())
        result = self.invoke('policy-status', '--deployment', self.token)
        self.assertOK(result)
        self.assertIn('registration=present', result.stdout)
        self.assertOK(self.invoke('policy-remove', '--deployment', self.token, '--deployments-stopped'))
        self.assertFalse(list((self.root / 'state/registrations').iterdir()))

    def test_failed_ensure_retains_policy_and_prints_recovery(self):
        self.settings['ensure_exit'] = 42
        result = self.invoke()
        self.assertEqual(result.returncode, 42, result.stderr)
        self.assertIn('host-policy=ready runtime=failed-or-cancelled', result.stderr)
        self.assertIn(self.token, result.stderr)
        self.assertTrue((self.root / 'state/registrations' / ('deployment-' + self.token)).is_file())
        self.settings.clear()
        self.assertOK(self.invoke())

    def test_policy_load_failure_preserves_registration_and_does_not_ensure(self):
        self.settings['policy_exit'] = 43
        result = self.invoke()
        self.assertEqual(result.returncode, 43, result.stderr)
        self.assertIn('host-policy=attempted runtime=not-attempted', result.stderr)
        self.assertFalse((self.root / 'ensured').exists())

    def test_replacement_on_completion_reports_prepared_old_target(self):
        self.settings['swapped'] = True
        result = self.invoke()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('runtime=prepared', result.stderr)
        self.assertIn('retry the same command', result.stderr)

    def test_host_without_restriction_skips_policy(self):
        (self.root / 'restrict').write_text('0\n')
        result = self.invoke()
        self.assertOK(result)
        self.assertIn('host-policy=not-required runtime=prepared', result.stderr)
        self.assertFalse((self.root / 'state').exists())

    def test_apparmor_enabled_kernel_without_restriction_knob_skips_policy(self):
        (self.root / 'restrict').unlink()
        result = self.invoke()
        self.assertOK(result)
        self.assertIn('host-policy=not-required runtime=prepared', result.stderr)
        self.assertFalse((self.root / 'state').exists())

    def test_invalid_identity_plan_confinement_and_storage_refuse_before_policy(self):
        cases = [
            {'identity': '0:202'}, {'identity': '201:0'}, {'identity': '201:202\nextra'},
            {'plan': 'netdata-dem-runtime-setup-v1\nstandalone\nmanaged\n/attacker'},
            {'invocation': '["/attacker"]|null|/attacker|[]||unconfined|false|'},
            {'invocation': '["/usr/sbin/run.sh"]|null|/usr/sbin/run.sh|[]||docker-default|false|'},
            {'security': '["name=rootless"]|Ubuntu 24.04'}, {'fstype': 'nfs'},
            {'volume': 'local|local|{"type":"nfs"}'}, {'mounts': []},
            {'mounts': [['bind', str(self.root / 'data'), '/usr/libexec', True, '']]},
        ]
        for settings in cases:
            with self.subTest(settings=settings):
                self.settings = settings
                result = self.invoke()
                self.assertNotEqual(result.returncode, 0, result.stderr)
                self.assertFalse((self.root / 'state').exists())
                self.assertFalse((self.root / 'ensured').exists())

    def test_oversized_target_output_is_bounded_without_abandoning_ensure(self):
        self.settings['identity'] = 'x' * 1000000
        result = self.invoke()
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse((self.root / 'state').exists())
        self.settings = {'ensure_output': 1000000}
        result = self.invoke()
        self.assertOK(result)
        self.assertEqual(len(result.stdout), 65536)
        self.assertIn('output truncated', result.stderr)

    def test_target_terminal_controls_are_removed_from_diagnostics_and_report(self):
        payload = 'safe\ttext\n\x1b]52;c;clipboard\x07\x9b31m'
        self.settings = {'diagnostic': payload, 'ensure_text': payload}
        result = self.invoke()
        self.assertOK(result)
        for output in (result.stdout, result.stderr):
            self.assertIn('safe\ttext\n', output)
            self.assertNotIn('\x1b', output)
            self.assertNotIn('\x07', output)
            self.assertNotIn('\x9b', output)
            self.assertTrue(all(c in '\t\n' or 32 <= ord(c) <= 126 for c in output))

    def test_apparmor_absent_does_not_require_apparmor_process_label(self):
        (self.root / 'aa').unlink()
        (self.root / 'context').unlink()
        result = self.invoke()
        self.assertOK(result)
        self.assertIn('host-policy=not-required', result.stderr)

    def kickstart_process(self, *args, env=None):
        source = (INSTALLER / 'kickstart.sh').read_text()
        for old, new in {
            '  PATH=/usr/sbin:/usr/bin:/sbin:/bin': '  PATH=' + str(self.bin) + ':/usr/sbin:/usr/bin:/sbin:/bin',
            '/usr/bin/docker': str(self.bin / 'docker'),
            '/.dockerenv': str(self.root / 'nested'),
            '/run/.containerenv': str(self.root / 'containerenv'),
            '/proc/$dem_pid': str(self.root / 'proc/$dem_pid'),
            'dem_trusted_dir /run ': 'dem_trusted_dir ' + str(self.root / 'run') + ' ',
            '[ ! -L /var/run/docker.sock ]': '[ ! -L ' + str(self.root / 'run/docker.sock') + ' ]',
            'for dem_dir in / /var /var/lib;': 'for dem_dir in ' + str(self.root / 'run') + ';',
            '[ -S /var/run/docker.sock ]': '[ -f ' + str(self.root / 'run/docker.sock') + ' ]',
            '/var/run/docker.sock)': str(self.root / 'run/docker.sock') + ')',
            'readlink -f -- /var/run)': 'readlink -f -- ' + str(self.root / 'var-run') + ')',
            '" = /run ]': '" = ' + str(self.root / 'run') + ' ]',
            'mktemp -d /run/netdata-dem-kickstart.': 'mktemp -d ' + str(self.root / 'run/kickstart.'),
            'dem_state=/var/lib/netdata-dem-runtime-host-policy': 'dem_state=' + str(self.root / 'state'),
        }.items():
            self.assertIn(old, source)
            source = source.replace(old, new)
        artifact = self.root / 'standalone-kickstart'
        artifact.write_text(source)
        (self.root / 'settings').write_text(json.dumps(self.settings))
        return subprocess.Popen(['sh', str(artifact), *args], cwd=self.root, env=env,
                                stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)

    def kickstart(self, *args):
        with self.kickstart_process(*args) as process:
            stdout, stderr = process.communicate(timeout=30)
            return subprocess.CompletedProcess(process.args, process.returncode, stdout, stderr)

    def test_kickstart_cancellation_reaches_privileged_helper_and_waits_for_drain(self):
        self.settings['block'] = True
        # Model a privilege boundary that preserves stdin but cannot forward the caller's signal.
        wrapper = self.bin / 'privilege-command'
        wrapper.write_text(f'#!{sys.executable}\nimport os, signal, subprocess, sys\n'
                           'args = sys.argv[1:]\n'
                           'if "docker-install" not in args: os.execvp(args[0], args)\n'
                           'signal.signal(signal.SIGTERM, lambda *_: None)\n'
                           'sys.exit(subprocess.call(args))\n')
        wrapper.chmod(0o755)
        env = dict(os.environ, ROOTCMD=str(wrapper))
        with self.kickstart_process('--install-dem-runtime', '--docker-container', 'monitor', env=env) as process:
            deadline = time.monotonic() + 20
            while not (self.root / 'ensured').exists() and process.poll() is None:
                if time.monotonic() > deadline:
                    process.kill()
                    self.fail('enrollment did not reach the supervised ensure operation')
                time.sleep(0.02)
            process.send_signal(signal.SIGTERM)
            stdout, stderr = process.communicate(timeout=10)
            self.assertEqual(process.returncode, 23, stderr)
        self.assertTrue((self.root / 'cancelled').is_file())
        self.assertFalse(list((self.root / 'run').glob('kickstart.*')))

    def test_standalone_enrollment_and_retirement_without_image_or_daemon(self):
        self.assertOK(self.kickstart('--install-dem-runtime', '--docker-container', 'monitor'))
        self.assertFalse((self.root / 'scratch').exists())
        self.assertFalse(list((self.root / 'run').glob('kickstart.*')))
        cached = self.root / 'state/docker-helpers' / ('deployment-' + self.token)
        self.assertEqual(cached.read_bytes(), self.script.read_bytes())
        self.settings['no_daemon'] = True
        (self.root / 'netdata-dem-docker-host.sh').unlink()
        self.script.unlink()
        status = self.kickstart('--dem-docker-policy-status', self.token)
        self.assertOK(status)
        self.assertIn('registration=present', status.stdout)
        self.assertOK(self.kickstart('--dem-docker-policy-remove', self.token, '--deployments-stopped'))
        self.assertFalse(cached.exists())
        self.assertIn('registration=absent', self.kickstart('--dem-docker-policy-status', self.token).stdout)

    def test_image_copy_failure_symlink_or_name_reuse_never_enroll(self):
        for settings, expected in [({'create_exit': 46}, 46), ({'copy_exit': 47}, 47),
                                   ({'link': True}, 1), ({'replaced': True}, 1)]:
            with self.subTest(settings=settings):
                self.settings = settings
                result = self.kickstart('--install-dem-runtime', '--docker-container', 'monitor')
                self.assertEqual(result.returncode, expected, result.stderr)
                self.assertFalse((self.root / 'scratch').exists())
                self.assertFalse(list((self.root / 'run').glob('kickstart.*')))
                self.assertFalse((self.root / 'state').exists())
                self.assertFalse((self.root / 'ensured').exists())

    def test_unverified_locality_and_hardened_host_refuse(self):
        (self.root / 'proc/123/cgroup').write_text('0::/remote\n')
        result = self.invoke()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('cannot correlate', result.stderr)
        (self.root / 'proc/123/cgroup').write_text('0::/docker/' + CONTAINER_ID + '\n')
        (self.root / 'unconfined').write_text('1\n')
        result = self.invoke()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('hardened', result.stderr)
        self.assertFalse((self.root / 'state').exists())

    def test_cancellation_closes_control_and_waits_for_target(self):
        self.settings['block'] = True
        (self.root / 'settings').write_text(json.dumps(self.settings))
        process = subprocess.Popen(['sh', str(self.script), 'docker-install', '--container', 'monitor',
                                    '--container-id', CONTAINER_ID],
                                   stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        self.addCleanup(lambda: process.poll() is None and process.kill())
        deadline = time.monotonic() + 15
        while not (self.root / 'ensured').exists() and process.poll() is None:
            self.assertLess(time.monotonic(), deadline)
            time.sleep(0.02)
        process.send_signal(signal.SIGTERM)
        stdout, stderr = process.communicate(timeout=10)
        self.assertEqual(process.returncode, 23, stderr)
        self.assertTrue((self.root / 'cancelled').exists())
        self.assertIn('failed-or-cancelled', stderr)


class StandaloneKickstartTests(unittest.TestCase):
    def test_standalone_artifact_refuses_conflicting_options_before_discovery(self):
        with tempfile.TemporaryDirectory() as temp:
            artifact = Path(temp) / 'standalone-kickstart.sh'
            artifact.write_bytes((INSTALLER / 'kickstart.sh').read_bytes())
            for args in [
                ['--install-dem-runtime', '--docker-container', 'monitor', '--claim-token', 'secret'],
                ['--docker-container', 'monitor'],
                ['--dem-docker-policy-remove', 'docker-token'],
            ]:
                result = subprocess.run(['sh', str(artifact), *args],
                                        cwd=temp, capture_output=True, text=True, timeout=5)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn('kickstart Docker setup:', result.stderr)
                self.assertNotIn('secret', result.stderr)

    def test_standalone_artifact_refuses_redirected_client(self):
        result = subprocess.run(['sh', str(INSTALLER / 'kickstart.sh'), '--install-dem-runtime',
                                 '--docker-container', 'monitor'], env=dict(os.environ, DOCKER_HOST='ssh://remote'),
                                capture_output=True, text=True, timeout=5)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('client overrides', result.stderr)


if __name__ == '__main__':
    unittest.main()

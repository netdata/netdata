# SPDX-License-Identifier: GPL-3.0-or-later
"""Observable maintscript tests; all privileged/system commands are stubbed."""
import json
import os
from pathlib import Path
import subprocess
import shutil
import sys
import tempfile
import unittest

SOURCE = Path(__file__).resolve().parents[1]
STUB = r'''#!/usr/bin/env python3
import json, os, pathlib, sys
root = pathlib.Path(os.environ['FIXTURE'])
state_path = root / 'state.json'
state = json.loads(state_path.read_text())
name = pathlib.Path(sys.argv[0]).name
args = sys.argv[1:]
with (root / 'calls.jsonl').open('a') as log:
    log.write(json.dumps([name, args]) + '\n')
if name == 'id':
    print('0'); sys.exit(0)
if name == 'stat':
    print('0:700' if pathlib.Path(args[-1]).is_dir() else '0:600'); sys.exit(0)
if name == 'systemctl' and 'show' in args:
    print(state.get('load', 'loaded') if '--property=LoadState' in args else state['active'])
    sys.exit(state.get('query_exit', 0))
if name in ('systemctl', 'deb-systemd-invoke'):
    action = next((a for a in args if a in ('start','stop')), None)
    code = state.get(action + '_exit', 0)
    if code: sys.exit(code)
    if not state.get('deny_' + action, False):
        state['active'] = 'active' if action == 'start' else 'inactive'
        state_path.write_text(json.dumps(state))
    sys.exit(0)
if name == 'apparmor_parser':
    code = state.get('parser_exit', 0)
    if not code:
        (root/'profiles').write_text('netdata-dem-browser (unconfined)\n' if '-r' in args else '')
    sys.exit(code)
if name == 'dem plugin':
    verb = args[-1]
    if verb == 'native-prepare':
        code = state.get('prepare_exit', 0)
        if not code: state['gate'] = 'closed'
    else:
        code = state.get('configure_exit', 0)
        if not code: state['gate'] = 'ready'
    state_path.write_text(json.dumps(state)); sys.exit(code)
print('unexpected stub invocation', name, args, file=sys.stderr)
sys.exit(99)
'''


class LifecycleTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="native hook 'test ")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.bin = self.root / 'bin'
        self.bin.mkdir()
        for name in ('id', 'stat', 'systemctl', 'deb-systemd-invoke', 'dem plugin', 'apparmor_parser'):
            p = self.bin / name
            p.write_text(STUB)
            p.chmod(0o755)
        self.systemd = self.root / 'systemd'
        self.systemd.mkdir()
        self.intent = self.root / 'intent'
        self.enabled = self.root / 'apparmor-enabled'
        self.enabled.write_text('N\n')
        (self.root/'profiles').write_text('')
        (self.root/'profile').write_text('fixture profile')
        self.state(active='active', gate='ready')

    def state(self, **changes):
        p = self.root / 'state.json'
        value = json.loads(p.read_text()) if p.exists() else {}
        value.update(changes)
        p.write_text(json.dumps(value))
        return value

    def calls(self):
        p = self.root / 'calls.jsonl'
        return [json.loads(line) for line in p.read_text().splitlines()] if p.exists() else []

    def hook(self, kind, action, family='deb'):
        # Arguments are positional, never interpreted as shell source.
        script = ('. "$1"; NATIVE_HELPER=$2; NATIVE_INTENT_DIR=$3; NATIVE_SYSTEMD_DIR=$4; '
                  'NATIVE_FAMILY=$5; NATIVE_APPARMOR_ENABLED=$8; NATIVE_APPARMOR_PROFILES=$9; shift 9; '
                  'NATIVE_APPARMOR_PROFILE=$1; NATIVE_APPARMOR_DISABLED=$2; native_hook "$3" "$4"')
        env = dict(os.environ, FIXTURE=str(self.root), PATH=str(self.bin)+os.pathsep+os.environ['PATH'])
        if getattr(self, 'isolated_path', False):
            env['PATH'] = str(self.bin)
        env.pop('DPKG_ROOT', None)
        return subprocess.run(['sh', '-c', script, 'fixture', str(SOURCE/'lifecycle.sh'),
                               str(self.bin/'dem plugin'), str(self.intent), str(self.systemd),
                               family, kind, action, str(self.enabled), str(self.root/'profiles'),
                               str(self.root/'profile'), str(self.root/'disabled'), kind, action],
                              env=env, text=True, capture_output=True)

    def marker(self):
        return self.intent / 'restart-pending'

    def test_upgrade_retries_preserve_original_active_intent(self):
        self.assertEqual(self.hook('deb-prerm', 'upgrade').returncode, 0)
        self.assertEqual(self.state()['active'], 'inactive')
        self.assertEqual(self.marker().read_text().strip(), 'active')
        self.assertEqual(self.hook('deb-preinst', 'upgrade').returncode, 0)
        self.assertEqual(self.marker().read_text().strip(), 'active')
        result = self.hook('deb-postinst', 'configure')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.state()['active'], 'active')
        self.assertEqual(self.state()['gate'], 'ready')
        self.assertFalse(self.marker().exists())
        self.assertFalse(any(name == 'systemctl' and 'stop' in args for name, args in self.calls()))

    def test_inactive_stays_inactive(self):
        self.state(active='inactive')
        self.assertEqual(self.hook('rpm-pre', '1', 'rpm').returncode, 0)
        self.assertEqual(self.hook('rpm-posttrans', '1', 'rpm').returncode, 0)
        self.assertEqual(self.state()['active'], 'inactive')
        self.assertFalse(any('start' in args for _, args in self.calls()))

    def test_policy_denied_stop_refuses_before_helper(self):
        self.state(deny_stop=True)
        result = self.hook('deb-preinst', 'upgrade')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('payload mutation refused', result.stderr)
        self.assertEqual(self.state()['gate'], 'ready')
        self.assertFalse(any(name == 'dem plugin' for name, _ in self.calls()))

    def test_service_stop_failure_preserves_exit_status(self):
        self.state(stop_exit=23)
        result = self.hook('deb-preinst', 'upgrade')
        self.assertEqual(result.returncode, 23)
        self.assertIn('status 23', result.stderr)
        self.assertIn(str(Path.cwd()), result.stderr)

    def test_busy_helper_restores_old_service_and_preserves_status(self):
        self.state(prepare_exit=29)
        result = self.hook('deb-preinst', 'upgrade')
        self.assertEqual(result.returncode, 29)
        self.assertEqual(self.state()['active'], 'active')
        self.assertEqual(self.state()['gate'], 'ready')

    def test_failed_qualification_restores_rum_and_retry_activates(self):
        self.state(configure_exit=31)
        result = self.hook('deb-postinst', 'configure')
        self.assertEqual(result.returncode, 31)
        self.assertEqual(self.state()['gate'], 'closed')
        self.assertEqual(self.state()['active'], 'active')
        self.assertFalse(self.marker().exists())
        self.state(configure_exit=0)
        self.assertEqual(self.hook('deb-postinst', 'configure').returncode, 0)
        self.assertEqual(self.state()['gate'], 'ready')

    def test_failed_qualification_status_survives_restore_failure(self):
        self.state(configure_exit=31, start_exit=37)
        result = self.hook('rpm-posttrans', '1', 'rpm')
        self.assertEqual(result.returncode, 31)
        self.assertTrue(self.marker().exists())
        self.assertEqual(self.state()['gate'], 'closed')

    def test_policy_denied_start_is_explicit_and_not_deferred(self):
        self.assertEqual(self.hook('deb-preinst', 'upgrade').returncode, 0)
        self.state(deny_start=True)
        result = self.hook('deb-postinst', 'configure')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('administrator restart', result.stderr)
        self.assertFalse(self.marker().exists())

    def test_final_remove_restores_before_erasure_and_postun_does_nothing(self):
        result = self.hook('rpm-preun', '0', 'rpm')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.state()['gate'], 'closed')
        self.assertEqual(self.state()['active'], 'active')
        count = len(self.calls())
        self.assertEqual(self.hook('rpm-postun', '0', 'rpm').returncode, 0)
        self.assertEqual(len(self.calls()), count)
        self.assertEqual(self.hook('rpm-preun', '1', 'rpm').returncode, 0)
        self.assertEqual(len(self.calls()), count)

    def test_abort_restores_without_qualifying_or_reopening(self):
        self.assertEqual(self.hook('deb-preinst', 'upgrade').returncode, 0)
        self.assertEqual(self.hook('deb-postinst', 'abort-upgrade').returncode, 0)
        self.assertEqual(self.state()['gate'], 'closed')
        self.assertEqual(self.state()['active'], 'active')
        self.assertFalse(any('native-configure' in args for _, args in self.calls()))

    def test_unknown_and_transitional_states_refuse(self):
        for state in ('activating', 'deactivating', 'reloading', 'unknown', ''):
            with self.subTest(state=state):
                self.state(active=state)
                result = self.hook('deb-preinst', 'install')
                self.assertNotEqual(result.returncode, 0)
                self.assertIn('ActiveState', result.stderr)
        self.assertFalse(any(name == 'dem plugin' for name, _ in self.calls()))

    def test_failed_service_is_stopped_and_proved_inactive_without_starting(self):
        self.state(active='failed')
        result = self.hook('deb-preinst', 'install')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.state()['active'], 'inactive')
        self.assertEqual(self.marker().read_text().strip(), 'inactive')
        self.assertEqual(self.hook('deb-postinst', 'configure').returncode, 0)
        self.assertEqual(self.state()['gate'], 'ready')
        self.assertTrue(any('stop' in args for _, args in self.calls()))
        self.assertFalse(any('start' in args for _, args in self.calls()))

    def test_denied_stop_of_failed_service_does_not_reach_payload_helper(self):
        self.state(active='failed', deny_stop=True)
        result = self.hook('deb-preinst', 'install')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('payload mutation refused', result.stderr)
        self.assertFalse(any(name == 'dem plugin' for name, _ in self.calls()))

    def test_chroot_and_symlink_intent_fail_closed(self):
        self.systemd.rmdir()
        result = self.hook('deb-preinst', 'install')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('chroot/image', result.stderr)
        self.systemd.mkdir()
        self.intent.symlink_to(self.systemd)
        result = self.hook('deb-preinst', 'install')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('plain root-owned', result.stderr)

    def test_apparmor_load_is_between_drain_and_qualification(self):
        self.enabled.write_text('Y\n')
        result = self.hook('deb-postinst', 'configure')
        self.assertEqual(result.returncode, 0, result.stderr)
        events = [(name, args[-1]) for name, args in self.calls() if name in ('dem plugin', 'apparmor_parser')]
        self.assertEqual(events, [('dem plugin', 'native-prepare'), ('apparmor_parser', str(self.root/'profile')),
                                  ('dem plugin', 'native-configure')])
        self.assertIn(['-r', str(self.root/'profile')], [args for name,
                      args in self.calls() if name == 'apparmor_parser'])

    def test_apparmor_load_failure_restores_rum_and_preserves_closed_gate(self):
        self.enabled.write_text('Y\n')
        self.state(parser_exit=43)
        result = self.hook('deb-postinst', 'configure')
        self.assertEqual(result.returncode, 43, result.stderr)
        self.assertEqual(self.state()['gate'], 'closed')
        self.assertEqual(self.state()['active'], 'active')
        self.assertFalse(any('native-configure' in args for _, args in self.calls()))
        self.state(parser_exit=0)
        self.assertEqual(self.hook('deb-postinst', 'configure').returncode, 0)

    def test_apparmor_remove_retry_after_unload_and_failed_restore(self):
        self.enabled.write_text('Y\n')
        (self.root/'profiles').write_text('netdata-dem-browser (unconfined)\n')
        self.state(start_exit=37)
        result = self.hook('deb-prerm', 'remove')
        self.assertEqual(result.returncode, 37, result.stderr)
        self.assertEqual(self.state()['gate'], 'closed')
        self.assertEqual((self.root/'profiles').read_text(), '')
        self.state(start_exit=0)
        self.assertEqual(self.hook('deb-prerm', 'remove').returncode, 0)
        self.assertEqual(len([1 for name, args in self.calls() if name == 'apparmor_parser']), 1)

    def test_apparmor_unload_failure_refuses_removal_restores_rum(self):
        self.enabled.write_text('Y\n')
        (self.root/'profiles').write_text('netdata-dem-browser (unconfined)\n')
        self.state(parser_exit=47)
        result = self.hook('rpm-preun', '0', 'rpm')
        self.assertEqual(result.returncode, 47, result.stderr)
        self.assertEqual(self.state()['gate'], 'closed')
        self.assertEqual(self.state()['active'], 'active')
        self.state(parser_exit=0)
        self.assertEqual(self.hook('rpm-preun', '0', 'rpm').returncode, 0)

    def test_apparmor_upgrade_posthooks_never_unload(self):
        self.enabled.write_text('Y\n')
        (self.root/'profiles').write_text('netdata-dem-browser (unconfined)\n')
        for hook, action, family in [('deb-postrm', 'upgrade', 'deb'),
                                     ('rpm-preun', '1', 'rpm'), ('rpm-postun', '1', 'rpm')]:
            self.assertEqual(self.hook(hook, action, family).returncode, 0)
        self.assertFalse(any(name == 'apparmor_parser' for name, _ in self.calls()))

    def test_apparmor_busy_consumer_prevents_policy_mutation(self):
        self.enabled.write_text('Y\n')
        self.state(prepare_exit=29)
        for hook, action in [('deb-postinst', 'configure'), ('deb-prerm', 'remove')]:
            self.assertEqual(self.hook(hook, action).returncode, 29)
        self.assertFalse(any(name == 'apparmor_parser' for name, _ in self.calls()))

    def test_apparmor_unreadable_inventory_refuses_removal(self):
        self.enabled.write_text('Y\n')
        (self.root/'profiles').unlink()
        result = self.hook('deb-prerm', 'remove')
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.state()['gate'], 'closed')
        self.assertEqual(self.state()['active'], 'active')
        self.assertFalse(any(name == 'apparmor_parser' for name, _ in self.calls()))

    def test_active_apparmor_missing_parser_refuses_with_rum_restored(self):
        self.enabled.write_text('Y\n')
        (self.bin/'apparmor_parser').unlink()
        for tool in ('sh', 'sed', 'mkdir', 'cat', 'rm'):
            (self.bin/tool).symlink_to(shutil.which(tool))
        (self.bin/'python3').symlink_to(sys.executable)
        self.isolated_path = True
        result = self.hook('deb-postinst', 'configure')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('requires apparmor_parser', result.stderr)
        self.assertEqual(self.state()['gate'], 'closed')
        self.assertEqual(self.state()['active'], 'active')

    def test_inactive_apparmor_needs_no_parser_or_policy(self):
        (self.bin/'apparmor_parser').unlink()
        (self.root/'profile').unlink()
        self.assertEqual(self.hook('deb-postinst', 'configure').returncode, 0)
        self.assertEqual(self.hook('deb-prerm', 'remove').returncode, 0)

    def test_administrator_disabled_profile_refuses_configuration(self):
        self.enabled.write_text('Y\n')
        (self.root/'disabled').symlink_to(self.root/'profile')
        result = self.hook('deb-postinst', 'configure')
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.state()['gate'], 'closed')
        self.assertFalse(any(name == 'apparmor_parser' for name, _ in self.calls()))

    def test_command_quoting_keeps_metacharacters_as_data(self):
        result = self.hook('deb-preinst', 'install')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual([args for name, args in self.calls() if name == 'dem plugin'],
                         [['runtime', 'native-prepare']])
        self.assertIn("'\\''", result.stderr)
        self.assertEqual(result.stdout, '')

    def test_command_diagnostics_redact_unexpected_secret_arguments(self):
        sink = self.root / 'argument sink'
        sink.write_text('#!/bin/sh\nprintf "%s" "$1" > "$2"\nexit 17\n')
        sink.chmod(0o755)
        output = self.root / 'recorded'
        secret = 'synthetic-token=not-for-logs;$(not-a-command)'
        result = subprocess.run(['sh', '-c', '. "$1"; native_run "$2" "$3" "$4"',
                                 'fixture', str(SOURCE/'lifecycle.sh'), str(sink), secret, str(output)],
                                capture_output=True, text=True)
        self.assertEqual(result.returncode, 17)
        self.assertEqual(output.read_text(), secret)
        self.assertNotIn(secret, result.stderr)
        self.assertIn('<redacted>', result.stderr)
        self.assertIn('status 17', result.stderr)


if __name__ == '__main__':
    unittest.main()

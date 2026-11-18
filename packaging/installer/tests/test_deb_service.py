"""Execute the Agent postinst with isolated package and service commands."""

import os
from pathlib import Path
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[3]
POSTINST = ROOT / 'packaging/cmake/pkg-files/deb/netdata/postinst'


class AgentServiceTests(unittest.TestCase):
    def run_postinst(self, arguments, *, active=False, policy_denied=False,
                     offline=False, systemd=True, previously_removed=False):
        with tempfile.TemporaryDirectory(prefix='netdata-deb-service-') as temporary:
            root = Path(temporary)
            commands = root / 'bin'
            commands.mkdir()
            state = root / 'state'
            state.write_text('active' if active else 'inactive')
            events = root / 'events'
            events.touch()
            systemd_directory = root / 'systemd'
            if systemd:
                systemd_directory.mkdir()
            # Redirect only filesystem probes; execute the actual hook logic.
            script = root / 'postinst'
            script.write_text(POSTINST.read_text().replace(
                '/run/systemd/system', str(systemd_directory)))
            for name in ('dpkg-statoverride', 'chown', 'chmod', 'grep', 'deb-systemd-helper'):
                command = commands / name
                command.write_text('#!/bin/sh\nexit 0\n')
                command.chmod(0o755)
            service = commands / 'systemctl'
            service.write_text('''#!/bin/sh
case " $* " in
    *" is-enabled "*) printf '%s\n' "$TEST_ENABLEMENT"; exit 0 ;;
    *" is-active "*) test "$(cat "$TEST_STATE")" = active; exit $? ;;
    *" daemon-reload "*) exit 0 ;;
esac
exit 1
''')
            service.chmod(0o755)
            invoke = commands / 'deb-systemd-invoke'
            invoke.write_text('''#!/bin/sh
printf '%s\n' "$1" >> "$TEST_EVENTS"
test "$TEST_POLICY_DENIED" = yes && exit 0
case "$1" in
    start|restart) printf active > "$TEST_STATE" ;;
    try-restart) test "$(cat "$TEST_STATE")" != active || printf active > "$TEST_STATE" ;;
    *) exit 1 ;;
esac
''')
            invoke.chmod(0o755)
            result = subprocess.run(['sh', str(script), *arguments], text=True, capture_output=True,
                                    env=dict(os.environ, PATH=str(commands) + ':' + os.environ['PATH'],
                                             TEST_STATE=str(state), TEST_EVENTS=str(events),
                                             TEST_ENABLEMENT='masked' if previously_removed else 'enabled',
                                             TEST_POLICY_DENIED='yes' if policy_denied else 'no',
                                             DPKG_ROOT=str(root) if offline else ''))
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            return state.read_text(), events.read_text().splitlines()

    def test_first_install_starts(self):
        self.assertEqual(self.run_postinst(['configure', ''])[0], 'active')

    def test_upgrade_or_reinstall_preserves_stopped_service(self):
        self.assertEqual(self.run_postinst(['configure', '2.12.0'])[0], 'inactive')

    def test_active_upgrade_or_reinstall_requests_restart(self):
        state, actions = self.run_postinst(['configure', '2.12.0'], active=True)
        self.assertEqual(state, 'active')
        self.assertIn(actions, [['restart'], ['try-restart']])

    def test_first_install_respects_service_policy(self):
        self.assertEqual(self.run_postinst(['configure', ''], policy_denied=True)[0], 'inactive')

    def test_install_after_removal_starts_despite_retained_config_version(self):
        self.assertEqual(self.run_postinst(['configure', '2.12.0'], previously_removed=True)[0], 'active')

    def test_abort_callbacks_keep_existing_restart_behavior(self):
        for action in ('abort-upgrade', 'abort-deconfigure', 'abort-remove'):
            with self.subTest(action=action):
                self.assertEqual(self.run_postinst([action])[0], 'active')

    def test_offline_or_non_systemd_does_not_control_service(self):
        for options in ({'offline': True}, {'systemd': False}):
            with self.subTest(options=options):
                self.assertEqual(self.run_postinst(['configure', ''], **options), ('inactive', []))

    def test_other_callbacks_do_not_control_service(self):
        self.assertEqual(self.run_postinst(['reconfigure']), ('inactive', []))


if __name__ == '__main__':
    unittest.main()

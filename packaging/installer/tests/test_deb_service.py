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
                     offline=False, systemd=True, query_failures=0, expected_queries=None,
                     warning_expected=False):
        with tempfile.TemporaryDirectory(prefix='netdata-deb-service-') as temporary:
            root = Path(temporary)
            commands = root / 'bin'
            commands.mkdir()
            state = root / 'state'
            state.write_text('active' if active else 'inactive')
            events = root / 'events'
            events.touch()
            failures = root / 'query-failures'
            failures.write_text(str(query_failures))
            queries = root / 'queries'
            queries.touch()
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
    *" is-active "*)
        printf 'query\n' >> "$TEST_QUERIES"
        remaining=$(cat "$TEST_QUERY_FAILURES")
        if [ "$remaining" -gt 0 ]; then
            printf '%s' "$((remaining - 1))" > "$TEST_QUERY_FAILURES"
            exit 1
        fi
        test "$(cat "$TEST_STATE")" != active || exit 0
        exit 3 ;;
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
                                             TEST_QUERY_FAILURES=str(failures), TEST_QUERIES=str(queries),
                                             TEST_POLICY_DENIED='yes' if policy_denied else 'no',
                                             DPKG_ROOT=str(root) if offline else ''))
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            if expected_queries is not None:
                self.assertEqual(len(queries.read_text().splitlines()), expected_queries)
            if warning_expected:
                self.assertIn('service activity query failed with status 1; restart skipped', result.stderr)
            else:
                self.assertNotIn('restart skipped', result.stderr)
            return state.read_text(), events.read_text().splitlines()

    def test_first_install_starts(self):
        self.assertEqual(self.run_postinst(['configure', ''])[0], 'active')

    def test_upgrade_or_reinstall_preserves_stopped_service(self):
        self.assertEqual(self.run_postinst(['configure', '2.12.0'], expected_queries=1)[0], 'inactive')

    def test_active_upgrade_or_reinstall_requests_restart(self):
        state, actions = self.run_postinst(['configure', '2.12.0'], active=True)
        self.assertEqual(state, 'active')
        self.assertIn(actions, [['restart'], ['try-restart']])

    def test_query_error_retries_active_service_with_restart_policy(self):
        self.assertEqual(self.run_postinst(['configure', '2.12.0'], active=True,
                                          query_failures=1, expected_queries=2),
                         ('active', ['restart']))

    def test_query_error_retry_preserves_stopped_service(self):
        self.assertEqual(self.run_postinst(['configure', '2.12.0'],
                                          query_failures=1, expected_queries=2),
                         ('inactive', []))

    def test_persistent_query_error_reports_skipped_restart(self):
        for active in (False, True):
            with self.subTest(active=active):
                self.assertEqual(self.run_postinst(['configure', '2.12.0'], active=active,
                                                  query_failures=2, expected_queries=2,
                                                  warning_expected=True),
                                 ('active' if active else 'inactive', []))

    def test_query_retry_keeps_debian_policy_in_control(self):
        self.assertEqual(self.run_postinst(['configure', '2.12.0'], active=True,
                                          query_failures=1, expected_queries=2, policy_denied=True),
                         ('active', ['restart']))

    def test_first_install_respects_service_policy(self):
        self.assertEqual(self.run_postinst(['configure', ''], policy_denied=True)[0], 'inactive')

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

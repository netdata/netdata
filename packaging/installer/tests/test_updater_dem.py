# SPDX-License-Identifier: GPL-3.0-or-later
"""Exercise native updater argv and failure handling without package installation."""
import os
from pathlib import Path
import re
import shlex
import subprocess
import tempfile
import unittest

UPDATER = Path(__file__).resolve().parents[1] / 'netdata-updater.sh'
FUNCTIONS = '\n'.join(re.findall(r'^([a-zA-Z_][a-zA-Z_0-9]*\s*\(\)\s*\{\n.*?^})',
                                 UPDATER.read_text(), re.M | re.S))


class NativePairUpdateTests(unittest.TestCase):
    def run_update(self, manager, enrolled=True, package_failure=0, admission_failure=0, removed=False):
        with tempfile.TemporaryDirectory(prefix='dem-native-updater-') as temporary:
            root = Path(temporary)
            calls = root / 'calls'
            marker = root / 'completed'
            helper = root / 'dem.plugin'
            helper.write_text('#!/bin/sh\nprintf "helper %s\\n" "$*" >> "$CALLS"\nexit "$ADMISSION_STATUS"\n')
            helper.chmod(0o755)
            package_manager = root / manager
            package_manager.write_text(
                '#!/bin/sh\nprintf "pm %s\\n" "$*" >> "$CALLS"\ntouch "$MARKER"\nexit "$PACKAGE_STATUS"\n')
            package_manager.chmod(0o755)
            query = '''#!/bin/sh
if [ "$ENROLLED" = 0 ] || { [ "$REMOVED" = 1 ] && [ -e "$MARKER" ]; }; then exit 1; fi
printf installed
'''
            for name in ('dpkg-query', 'rpm'):
                path = root / name
                path.write_text(query)
                path.chmod(0o755)
            script = FUNCTIONS.replace('/usr/libexec/netdata/plugins.d/dem.plugin', shlex.quote(str(helper)))
            script += '\nexec 3>&2\nenv=\npkg_install_opts=-y\nupgrade_subcmd=upgrade\n'
            script += f'pm_cmd={manager}\nINSTALL_TYPE=binpkg-{"deb" if manager == "apt-get" else "rpm"}\n'
            script += 'update_native_agent_packages\n'
            environment = dict(os.environ, PATH=str(root) + ':' + os.environ['PATH'], CALLS=str(calls),
                               MARKER=str(marker), ENROLLED=str(int(enrolled)), REMOVED=str(int(removed)),
                               PACKAGE_STATUS=str(package_failure), ADMISSION_STATUS=str(admission_failure))
            result = subprocess.run(['sh', '-c', script], env=environment, text=True, capture_output=True)
            return result, calls.read_text().splitlines() if calls.exists() else []

    def test_only_enrolled_installations_request_the_optional_pair(self):
        for manager in ('apt-get', 'dnf', 'yum', 'zypper'):
            for enrolled in (False, True):
                with self.subTest(manager=manager, enrolled=enrolled):
                    result, calls = self.run_update(manager, enrolled)
                    self.assertEqual(result.returncode, 0, result.stderr)
                    expected = 'pm upgrade -y '
                    if enrolled and manager == 'apt-get':
                        expected += '--no-remove '
                    expected += 'netdata'
                    if enrolled:
                        expected += ' netdata-plugin-dem netdata-dem-runtime'
                    self.assertEqual(calls, [expected, 'helper runtime native-status'] if enrolled else [expected])

    def test_package_failure_preserves_status_and_does_not_check_admission(self):
        result, calls = self.run_update('apt-get', package_failure=37)
        self.assertEqual(result.returncode, 37)
        self.assertEqual(len(calls), 1)

    def test_rpm_success_with_closed_admission_is_reported_as_failure(self):
        result, calls = self.run_update('dnf', admission_failure=38)
        self.assertEqual(result.returncode, 38)
        self.assertEqual(calls[-1], 'helper runtime native-status')

    def test_unexpected_runtime_removal_is_not_reported_as_success(self):
        result, calls = self.run_update('dnf', removed=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(len(calls), 1)


if __name__ == '__main__':
    unittest.main()

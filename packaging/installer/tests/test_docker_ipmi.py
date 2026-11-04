#!/usr/bin/env python3
"""Check Docker entrypoint group access for the dedicated IPMI plugin."""
import os
from pathlib import Path
import subprocess
import unittest

ROOT = Path(__file__).resolve().parents[3]


class DockerIPMITests(unittest.TestCase):
    def test_selected_user_can_execute_installed_ipmi_plugin(self):
        source = (ROOT / 'packaging/docker/run.sh').read_text()
        helpers = source[source.index('# Check if user is a member'):source.index('if [ "${DISABLE_TELEMETRY')]
        setup = source[source.index('if [ "${EUID}" -eq 0 ]; then'):source.index('if mountpoint -q /etc/netdata; then')]
        setup = setup.replace('"${EUID}"', '"${TEST_EUID}"')
        commands = '''
stat() {
  case "$3" in
    */ipmi.plugin) [ "$TEST_PLUGIN_PRESENT" = yes ] && printf '201\\n' ;;
    *) return 1 ;;
  esac
}
getent() { printf 'netdata:x:201:netdata\\n'; }
usermod() { printf 'usermod %s\\n' "$*"; }
'''
        for user, installed, root, expected in (
                ('custom-user', 'yes', '0', 'usermod --append --groups 201 custom-user'),
                ('netdata', 'yes', '0', None),
                ('root', 'yes', '0', None),
                ('custom-user', 'no', '0', None),
                ('custom-user', 'yes', '1000', None)):
            with self.subTest(user=user, installed=installed, root=root):
                environment = dict(os.environ, DOCKER_USR=user, TEST_PLUGIN_PRESENT=installed, TEST_EUID=root,
                                   NETDATA_EXTRA_APK_PACKAGES='', NETDATA_EXTRA_DEB_PACKAGES='', PGID='')
                result = subprocess.run(['bash', '-c', helpers + commands + setup],
                                        capture_output=True, text=True, env=environment)
                self.assertEqual(result.returncode, 0, result.stderr)
                operations = [line for line in result.stdout.splitlines() if line.startswith('usermod ')]
                self.assertEqual(operations, [expected] if expected else [])


if __name__ == '__main__':
    unittest.main()

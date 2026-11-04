# SPDX-License-Identifier: GPL-3.0-or-later
"""Execute the core maintscript and native lifecycle with isolated command stubs."""
import os
from pathlib import Path
import re
import subprocess
import unittest

import test_lifecycle as lifecycle


CORE_PRERM = Path(__file__).resolve().parents[3] / 'cmake/pkg-files/deb/netdata/prerm'


class AgentRemovalTests(unittest.TestCase):
    def setUp(self):
        # Share the existing service/helper fixture without inheriting its tests.
        self.fixture = lifecycle.LifecycleTests()
        self.fixture.setUp()
        self.addCleanup(self.fixture.doCleanups)
        self.root = self.fixture.root
        self.payload = self.root / 'agent-payload'
        self.payload.write_text('installed Agent files')
        self.updater = self.fixture.bin / 'updater'
        self.updater.write_text('''#!/usr/bin/env python3
import json, os, pathlib, sys
root = pathlib.Path(os.environ['FIXTURE'])
with (root / 'calls.jsonl').open('a') as log:
    log.write(json.dumps(['updater', sys.argv[1:]]) + '\\n')
sys.exit(json.loads((root / 'state.json').read_text()).get('updater_exit', 0))
''')
        self.updater.chmod(0o755)
        self.script = self.root / 'prerm'
        # Relocate only literal host paths. All maintscript control flow is the
        # actual package source; no root access or real service calls are needed.
        self.script.write_text(re.sub(
            r'"?(/run/systemd/system|/usr/libexec/netdata/netdata-updater.sh)"?',
            lambda match: '"$FIXTURE_SYSTEMD"' if match[1] == '/run/systemd/system' else '"$FIXTURE_UPDATER"',
            CORE_PRERM.read_text()))

    def core_hook(self, action='remove', dpkg_root=''):
        env = dict(os.environ, FIXTURE=str(self.root), DPKG_ROOT=dpkg_root,
                   FIXTURE_SYSTEMD=str(self.fixture.systemd), FIXTURE_UPDATER=str(self.updater),
                   PATH=str(self.fixture.bin) + os.pathsep + os.environ['PATH'])
        return subprocess.run(['sh', str(self.script), action], env=env,
                              capture_output=True, text=True)

    def remove_agent(self):
        result = self.core_hook()
        # Model dpkg's successful-prerm boundary before package-file erasure.
        if result.returncode == 0:
            self.assertEqual(self.fixture.state()['active'], 'inactive',
                             'core prerm allowed erasure while Agent was not inactive')
            self.payload.unlink()
        return result

    def native_remove(self):
        result = self.fixture.hook('deb-prerm', 'remove')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.fixture.state()['gate'], 'closed')
        self.assertEqual(self.fixture.state()['active'], 'active')

    def test_core_only_removal_stops_before_erasure(self):
        result = self.remove_agent()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse(self.payload.exists())
        self.assertIn(['updater', ['--disable-auto-updates']], self.fixture.calls())
        self.assertIn(['deb-systemd-invoke', ['stop', 'netdata.service']], self.fixture.calls())

    def test_combined_removal_stops_restored_agent_before_core_erasure(self):
        self.native_remove()
        result = self.remove_agent()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertFalse(self.payload.exists())
        self.assertEqual(self.fixture.state()['gate'], 'closed')
        actions = [args[0] for name, args in self.fixture.calls() if name == 'deb-systemd-invoke']
        self.assertEqual(actions, ['stop', 'start', 'stop'])

    def test_denied_or_failed_stop_prevents_core_erasure(self):
        for combined in (False, True):
            for changes, expected in (({'deny_stop': True}, 1), ({'stop_exit': 23}, 23)):
                with self.subTest(combined=combined, changes=changes):
                    self.fixture.state(active='active', deny_stop=False, stop_exit=0)
                    if combined:
                        self.native_remove()
                    self.fixture.state(**changes)
                    result = self.remove_agent()
                    self.assertEqual(result.returncode, expected, result.stderr)
                    self.assertIn('removal refused', result.stderr)
                    if expected == 23:
                        self.assertIn('status 23', result.stderr)
                    self.assertTrue(self.payload.exists())
                    self.assertEqual(self.fixture.state()['active'], 'active')

    def test_service_state_must_be_observed_inactive(self):
        self.fixture.state(query_exit=17)
        result = self.remove_agent()
        self.assertEqual(result.returncode, 17, result.stderr)
        self.assertIn('status 17; removal refused', result.stderr)
        self.assertTrue(self.payload.exists())
        for active in ('failed', 'deactivating', 'unknown', ''):
            with self.subTest(active=active):
                self.fixture.state(active=active, query_exit=0, deny_stop=True)
                result = self.remove_agent()
                self.assertNotEqual(result.returncode, 0)
                self.assertTrue(self.payload.exists())

    def test_inactive_and_failed_services_can_be_removed_after_stop(self):
        for active in ('inactive', 'failed'):
            with self.subTest(active=active):
                self.fixture.state(active=active)
                result = self.core_hook()
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(self.fixture.state()['active'], 'inactive')

    def test_upgrade_does_not_stop_or_disable_updates(self):
        result = self.core_hook('upgrade')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.fixture.state()['active'], 'active')
        self.assertEqual(self.fixture.calls(), [])

    def test_updater_failure_still_refuses_removal(self):
        self.fixture.state(updater_exit=31)
        result = self.remove_agent()
        self.assertEqual(result.returncode, 31, result.stderr)
        self.assertTrue(self.payload.exists())
        self.assertEqual(self.fixture.state()['active'], 'inactive')

    def test_chroot_and_non_systemd_keep_existing_updater_behavior(self):
        for chroot in (True, False):
            with self.subTest(chroot=chroot):
                if not chroot:
                    self.fixture.systemd.rmdir()
                result = self.core_hook(dpkg_root='/image' if chroot else '')
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(self.fixture.state()['active'], 'active')
        self.assertEqual(self.fixture.calls(), [
            ['updater', ['--disable-auto-updates']],
            ['updater', ['--disable-auto-updates']],
        ])


if __name__ == '__main__':
    unittest.main()

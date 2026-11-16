# SPDX-License-Identifier: GPL-3.0-or-later
"""Exercise the image adapter called by host kickstart without a daemon."""
import os
from pathlib import Path
import pwd
import subprocess
import tempfile
import unittest


DOCKER = Path(__file__).resolve().parents[2] / 'docker'


class ServiceAdapter(unittest.TestCase):
    def test_actual_identity_and_service_cli_arguments(self):
        account = pwd.getpwuid(os.geteuid()) if os.geteuid() else pwd.getpwnam('nobody')
        if account.pw_gid == 0:
            self.skipTest('test requires a nonroot UID and GID')
        with tempfile.TemporaryDirectory() as name:
            root = Path(name)
            root.chmod(0o755)
            agent, plugin = root / 'agent', root / 'dem.plugin'
            agent.write_text('#!/bin/sh\ncase "$6" in\n'
                             f'plugins) printf \'"%s" "%s/second"\\n\' "{root}" "{root}" ;;\n'
                             f'*) printf "%s\\n" "{root}" ;;\nesac\n')
            plugin.write_text('#!/bin/sh\nid -u\nprintf \'%s\\n\' "$HOME" "$@"\n')
            adapter = root / 'adapter'
            source = (DOCKER / 'netdata-dem-docker').read_text()
            adapter.write_text(source.replace('/usr/sbin/netdata', str(agent))
                               .replace('/usr/libexec/netdata/plugins.d', str(root)))
            for path in [agent, plugin, adapter]:
                path.chmod(0o755)
            credentials = (
                {'user': account.pw_uid, 'group': account.pw_gid, 'extra_groups': []}
                if os.geteuid() == 0 else {}
            )
            env = dict(os.environ, DOCKER_USR=account.pw_name)
            for args in [['identity'], ['plan'], ['ensure']]:
                with self.subTest(args=args):
                    r = subprocess.run([adapter, *args], env=env, **credentials, text=True,
                                       capture_output=True, timeout=10)
                    self.assertEqual(r.returncode, 0, r.stderr)
                    if args == ['identity']:
                        self.assertEqual(r.stdout.strip(), f'{account.pw_uid}:{account.pw_gid}')
                        continue
                    uid, home, *cli = r.stdout.splitlines()
                    self.assertEqual((int(uid), home), (account.pw_uid, str(root)))
                    if args == ['plan']:
                        self.assertEqual(cli, ['runtime', 'setup-plan', '--config-dir', str(root)])
                    else:
                        self.assertEqual(cli, ['runtime', 'ensure', '--allow-download', '--config-dir', str(root),
                                               '--expected-root', '/var/lib/netdata/dem/runtime',
                                               '--cancel-on-stdin-close'])

    def test_root_service_account_refused(self):
        result = subprocess.run(['bash', DOCKER / 'netdata-dem-docker', 'identity'],
                                env=dict(os.environ, DOCKER_USR='root'), capture_output=True, text=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(result.stdout, '')

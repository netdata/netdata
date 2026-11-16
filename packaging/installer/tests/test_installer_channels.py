# SPDX-License-Identifier: GPL-3.0-or-later
"""Exercise installer channel parsing and persistence without installing on the host."""
from pathlib import Path
import re
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[3]
SOURCE = (ROOT / 'netdata-installer.sh').read_text()
STATIC = (ROOT / 'packaging/makeself/install-or-update.sh').read_text()
FUNCTIONS = (ROOT / 'packaging/installer/functions.sh').read_text()


def parser(script):
    return re.search(r'^RELEASE_CHANNEL="nightly".*?^done$', script, re.M | re.S).group()


class InstallerChannels(unittest.TestCase):
    def test_channel_selection_and_saved_environment(self):
        cases = (
            ([], 'nightly'),
            (['--stable-channel'], 'stable'),
            (['--nightly-channel'], 'nightly'),
            (['--preview-channel'], 'preview'),
            (['--stable-channel', '--preview-channel'], 'preview'),
            (['--preview-channel', '--stable-channel'], 'stable'),
            (['--preview-channel', '--nightly-channel'], 'nightly'),
            (['--nightly-channel', '--preview-channel'], 'preview'),
        )
        for script, shell in ((SOURCE, '/bin/sh'), (STATIC, '/bin/bash')):
            for flags, expected in cases:
                with self.subTest(shell=shell, flags=flags), tempfile.TemporaryDirectory() as temp:
                    envfile = Path(temp) / '.environment'
                    setup = '''
set -e
REINSTALL_OPTIONS="$*"
NETDATA_PREFIX="$PWD"
NETDATA_USER_CONFIG_DIR="$PWD"
'''
                    if script == SOURCE:
                        persistence = re.search(
                            r'^cat << EOF > "\$\{NETDATA_USER_CONFIG_DIR\}/\.environment"\n.*?^EOF$',
                            SOURCE, re.M | re.S).group()
                    else:
                        # The archive contains an environment file from the build.
                        envfile.write_text('RELEASE_CHANNEL="stable"\nREINSTALL_OPTIONS=""\n')
                        setup += 'REINSTALL_OPTIONS=""\n'
                        persistence = re.search(
                            r'^set_netdata_updater_channel\(\) \{\n.*?^}',
                            FUNCTIONS, re.M | re.S).group()
                        persistence += '\nset_netdata_updater_channel\n'
                        persistence += next(line for line in STATIC.splitlines()
                                            if line.startswith('sed -i "s/REINSTALL_OPTIONS='))
                        # Redirect only the fixed install prefix in the real writer.
                        persistence = persistence.replace(
                            '${NETDATA_PREFIX}/etc/netdata/.environment',
                            '${NETDATA_USER_CONFIG_DIR}/.environment')
                    result = subprocess.run(
                        [shell, '-c', setup + parser(script) + '\n' + persistence, 'installer', *flags],
                        cwd=temp, text=True, capture_output=True)
                    self.assertEqual(result.returncode, 0, result.stderr)
                    self.assertEqual(result.stderr, '')
                    values = dict(line.split('=', 1) for line in envfile.read_text().splitlines()
                                  if '=' in line)
                    self.assertEqual(values['RELEASE_CHANNEL'], f'"{expected}"')
                    self.assertEqual(values['REINSTALL_OPTIONS'].strip('"').split(), flags)


if __name__ == '__main__':
    unittest.main()

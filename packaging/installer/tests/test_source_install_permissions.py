#!/usr/bin/env python3
"""Exercise the source install operation with a real, minimal CMake project."""
import os
from pathlib import Path
import shutil
import stat
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[3]


class SourceInstallFailureTests(unittest.TestCase):
    def test_install_preserves_warning_status_and_umask(self):
        installer = (ROOT / 'netdata-installer.sh').read_text()
        begin = installer.index('cmake_install()')
        end = installer.index('# shellcheck disable=SC2329', begin)
        with tempfile.TemporaryDirectory(prefix='netdata-install-failure-') as temp:
            script = Path(temp) / 'install.sh'
            script.write_text('''#!/bin/sh
set -e
. "$1"
NETDATA_SAVE_WARNINGS=1
SAVED_WARNINGS=existing
id() { printf '%s\\n' "$TEST_UID"; }
failing_install() { return "$TEST_STATUS"; }
make=failing_install
ninja=
[ "$TEST_BACKEND" != ninja ] || ninja=failing_install
umask 002
''' + installer[begin:end] + '''
if cmake_install build; then install_status=0; else install_status=$?; fi
printf '%s\\n%s\\n%s\\n' "$install_status" "$(umask)" "$SAVED_WARNINGS"
''')
            for uid in ('0', '1000'):
                for backend in ('make', 'ninja'):
                    for status in (0, 37):
                        with self.subTest(uid=uid, backend=backend, status=status):
                            result = subprocess.run(
                                ['sh', str(script), str(ROOT / 'packaging/installer/functions.sh')],
                                env=dict(os.environ, TEST_UID=uid,
                                         TEST_BACKEND=backend, TEST_STATUS=str(status)),
                                capture_output=True, text=True)
                            self.assertEqual(result.returncode, 0, result.stderr)
                            lines = result.stdout.splitlines()
                            self.assertEqual(lines[:2], [str(status), '0002'])
                            self.assertIn('existing', lines[2])
                            if status:
                                self.assertIn('failed with exit code 37', lines[2])


@unittest.skipUnless(sys.platform == 'linux' and shutil.which('cmake') and shutil.which('make'),
                     'Linux, CMake and make required')
class SourceInstallPermissionsTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='netdata-source-permissions-')
        self.addCleanup(self.temp.cleanup)
        self.base = Path(self.temp.name)
        self.base.chmod(0o755)
        self.source = self.base / 'source'
        self.source.mkdir()
        (self.source / 'tool').write_text('executable fixture\n')
        (self.source / 'data').write_text('data fixture\n')
        (self.source / 'CMakeLists.txt').write_text('''cmake_minimum_required(VERSION 3.16)
project(InstallPermissions NONE)
install(PROGRAMS tool DESTINATION usr/sbin)
install(FILES data DESTINATION usr/share/netdata)
''')
        installer = (ROOT / 'netdata-installer.sh').read_text()
        begin = installer.index('cmake_install()')
        end = installer.index('# shellcheck disable=SC2329', begin)
        self.script = self.base / 'install.sh'
        self.script.write_text('''#!/bin/sh
set -eu
run() { "$@"; }
make=make
ninja=
umask 002
''' + installer[begin:end] + '''
cmake_install "$1"
test "$(umask)" = 0002
''')

    def install(self, prefix, nonroot=False):
        build = self.base / 'build'
        build.mkdir()
        options = {}
        if nonroot and os.geteuid() == 0:
            os.chown(build, 65534, 65534)
            os.chown(prefix.parent, 65534, 65534)

            def drop_identity():
                os.setgroups([])
                os.setgid(65534)
                os.setuid(65534)

            options['preexec_fn'] = drop_identity
        configured = subprocess.run(['cmake', '-S', str(self.source), '-B', str(build),
                                     '-G', 'Unix Makefiles', '-DCMAKE_INSTALL_PREFIX=' + str(prefix)],
                                    capture_output=True, text=True, **options)
        self.assertEqual(configured.returncode, 0, configured.stdout + configured.stderr)
        installed = subprocess.run(['sh', str(self.script), str(build)],
                                   capture_output=True, text=True, **options)
        self.assertEqual(installed.returncode, 0, installed.stdout + installed.stderr)
        self.assertEqual(stat.S_IMODE((prefix / 'usr/sbin/tool').stat().st_mode), 0o755)
        self.assertEqual(stat.S_IMODE((prefix / 'usr/share/netdata/data').stat().st_mode), 0o644)

    @unittest.skipUnless(os.geteuid() == 0, 'root install contract')
    def test_fresh_root_install_creates_safe_executable_ancestors(self):
        prefix = self.base / 'new-prefix'
        self.install(prefix)
        for relative in ('.', 'usr', 'usr/sbin', 'usr/share', 'usr/share/netdata'):
            self.assertEqual(stat.S_IMODE((prefix / relative).stat().st_mode), 0o755, relative)

    @unittest.skipUnless(os.geteuid() == 0, 'root install contract')
    def test_root_install_preserves_existing_shared_parent_permissions(self):
        prefix = self.base / 'existing-prefix'
        shared = prefix / 'usr/sbin'
        shared.mkdir(parents=True)
        for path in (prefix, prefix / 'usr', shared):
            path.chmod(0o775)
        private = prefix / 'application/private'
        private.mkdir(parents=True, mode=0o700)
        self.install(prefix)
        for path in (prefix, prefix / 'usr', shared):
            self.assertEqual(stat.S_IMODE(path.stat().st_mode), 0o775)
        self.assertEqual(stat.S_IMODE(private.stat().st_mode), 0o700)
        self.assertEqual(stat.S_IMODE((prefix / 'usr/share').stat().st_mode), 0o755)

    def test_nonroot_install_preserves_existing_creation_policy(self):
        parent = self.base / 'user-install'
        parent.mkdir()
        prefix = parent / 'netdata'
        self.install(prefix, nonroot=True)
        for relative in ('.', 'usr', 'usr/sbin'):
            self.assertEqual(stat.S_IMODE((prefix / relative).stat().st_mode), 0o775, relative)


if __name__ == '__main__':
    unittest.main()

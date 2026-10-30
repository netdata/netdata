# SPDX-License-Identifier: GPL-3.0-or-later
"""Configuration-only recipe checks; no native package installation."""
import json
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

SOURCE = Path(__file__).resolve().parents[1]


@unittest.skipUnless(shutil.which('cmake'), 'requires CMake')
class RecipeTests(unittest.TestCase):
    def configure(self, root, arch='arm64', fmt='DEB', version='1.2.3',
                  staged_version='1.2.3', release='1'):
        payload = root / 'payload'
        payload.mkdir(exist_ok=True)
        (payload/'native.json').write_text(json.dumps({
            'format': 1, 'path': '/usr/lib/netdata/dem-runtime',
            'manifest': {'platform': 'linux/arm64', 'version': staged_version}}))
        build = root / 'build'
        result = subprocess.run(['cmake', '-S', str(SOURCE), '-B', str(build),
                                 '-DCMAKE_SYSTEM_NAME=Linux', '-DDEM_PAYLOAD_DIR='+str(payload),
                                 '-DDEM_ARCH='+arch, '-DDEM_VERSION='+version,
                                 '-DDEM_RELEASE='+release, '-DDEM_PACKAGE_FORMAT='+fmt],
                                capture_output=True, text=True)
        return result, build

    def test_hooks_and_capability_dependencies(self):
        for fmt in ('DEB','RPM'):
            with self.subTest(fmt=fmt), tempfile.TemporaryDirectory() as temp:
                result,build = self.configure(Path(temp),fmt=fmt)
                self.assertEqual(result.returncode,0,result.stdout+result.stderr)
                config = (build/'CPackConfig.cmake').read_text()
                self.assertIn('netdata-dem-runtime-api',config)
                self.assertIn('set(CPACK_STRIP_FILES "OFF")',config)
                if fmt == 'DEB':
                    self.assertIn('PACKAGE_PREDEPENDS "netdata-dem-runtime-api (= 1)',config)
                    hooks = ('preinst','postinst','prerm','postrm')
                else:
                    self.assertIn('Requires(posttrans): netdata-dem-runtime-api = 1',config)
                    self.assertIn('PACKAGE_ARCHITECTURE "aarch64"',config)
                    hooks = ('pre','preun','posttrans','postun')
                for hook in hooks:
                    script = build/'hooks'/hook
                    checked = subprocess.run(['sh','-n',str(script)],capture_output=True,text=True)
                    self.assertEqual(checked.returncode,0,checked.stderr)
                    self.assertIn('native_hook',script.read_text())
                    self.assertNotIn('@NATIVE_',script.read_text())
                install = (build/'cmake_install.cmake').read_text()
                self.assertIn('usr/lib/netdata/dem-runtime',install)
                self.assertNotIn('/var/lib/netdata-dem-runtime',install)

    def test_project_and_package_versions(self):
        versions = {'2026.10.29.1': '2026.10.29.1', '1.2.3~rc1': '1.2.3',
                    '1.2.3+build7': '1.2.3', '1.2.3.4.5': '1.2.3.4'}
        for fmt in ('DEB', 'RPM'):
            for version, project_version in versions.items():
                with self.subTest(fmt=fmt, version=version), tempfile.TemporaryDirectory() as temp:
                    root = Path(temp)
                    result, build = self.configure(root, fmt=fmt, version=version,
                                                   staged_version=version, release='7')
                    self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                    cache = (build / 'CMakeCache.txt').read_text().splitlines()
                    cached_version = next((line.split('=', 1)[1] for line in cache
                                           if line.startswith('CMAKE_PROJECT_VERSION:STATIC=')), None)
                    self.assertEqual(cached_version, project_version)
                    output = root / 'package-version'
                    inspect = root / 'inspect.cmake'
                    inspect.write_text('include("' + str(build / 'CPackConfig.cmake') + '")\n' +
                                       'file(WRITE "' + str(output) + '" "${CPACK_PACKAGE_VERSION}\\n' +
                                       '${CPACK_' + ('DEBIAN' if fmt == 'DEB' else 'RPM') +
                                       '_PACKAGE_RELEASE}")\n')
                    checked = subprocess.run(['cmake', '-P', str(inspect)], capture_output=True, text=True)
                    self.assertEqual(checked.returncode, 0, checked.stdout + checked.stderr)
                    self.assertEqual(output.read_text().splitlines(), [version, '7'])

    def test_requested_identity_must_match_staging(self):
        for kwargs in ({'arch':'amd64'},{'version':'9.9'},{'arch':'s390x'},{'version':'1;bad'}):
            with self.subTest(kwargs=kwargs), tempfile.TemporaryDirectory() as temp:
                result,build = self.configure(Path(temp),**kwargs)
                self.assertNotEqual(result.returncode,0)
                # Invalid package identities must fail before project initialization.
                cache = build / 'CMakeCache.txt'
                if cache.exists():
                    self.assertFalse(any(line.startswith('CMAKE_PROJECT_NAME:')
                                         for line in cache.read_text().splitlines()))


if __name__ == '__main__':
    unittest.main()

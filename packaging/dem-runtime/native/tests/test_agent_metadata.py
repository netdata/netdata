# SPDX-License-Identifier: GPL-3.0-or-later
"""Evaluate the Agent's real CPack configuration without building the Agent."""
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

REPO = Path(__file__).resolve().parents[4]


@unittest.skipUnless(shutil.which('cmake'), 'requires CMake')
class AgentMetadataTests(unittest.TestCase):
    def configure(self, root, distro, version, enabled=True):
        source, build = root / 'source', root / 'build'
        source.mkdir()
        for name in ('LICENSE', 'README.md'):
            (source / name).symlink_to(REPO / name)
        release = root / 'os-release'
        release.write_text(f'ID={distro}\nVERSION_ID={version}\n')
        (source / 'CMakeLists.txt').write_text('''
cmake_minimum_required(VERSION 3.21)
project(metadata-test LANGUAGES NONE)
list(APPEND CMAKE_MODULE_PATH "${REPO}/packaging/cmake/Modules")
set(OS_LINUX TRUE)
set(NETDATA_PACKAGE_VERSION 2.10.0)
set(PKG_FILES_PATH "${REPO}/packaging/cmake/pkg-files")
include(NetdataUtil)
include(Packaging)
''')
        result = subprocess.run([
            'cmake', '-S', str(source), '-B', str(build), '-DREPO=' + str(REPO),
            '-DOS_RELEASE_PATH=' + str(release), '-DENABLE_PLUGIN_DEM=' + ('ON' if enabled else 'OFF'),
        ], capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        keys = [
            'CPACK_DEBIAN_NETDATA_PACKAGE_DEPENDS', 'CPACK_DEBIAN_NETDATA_PACKAGE_RECOMMENDS',
            'CPACK_RPM_NETDATA_PACKAGE_REQUIRES', 'CPACK_RPM_NETDATA_PACKAGE_RECOMMENDS',
            'CPACK_DEBIAN_PLUGIN-DEM_PACKAGE_SUGGESTS', 'CPACK_RPM_PLUGIN-DEM_PACKAGE_SUGGESTS',
        ]
        inspect = root / 'inspect.cmake'
        inspect.write_text('include("' + str(build / 'CPackConfig.cmake') + '")\n' +
                           '\n'.join('file(APPEND "' + str(root / 'metadata') + '" "' +
                                     key + '=${' + key + '}\\n")' for key in keys))
        result = subprocess.run(['cmake', '-P', str(inspect)], capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        return dict(line.split('=', 1) for line in (root / 'metadata').read_text().splitlines())

    def test_dem_is_optional_with_runtime_suggestion(self):
        for distro, version, weak in [('debian', '12', True), ('rocky', '9', True),
                                      ('centos', '7', False), ('amzn', '2', False)]:
            with self.subTest(distro=distro), tempfile.TemporaryDirectory() as temp:
                metadata = self.configure(Path(temp), distro, version)
                self.assertNotIn('netdata-plugin-dem', metadata['CPACK_DEBIAN_NETDATA_PACKAGE_DEPENDS'])
                self.assertNotIn('netdata-plugin-dem', metadata['CPACK_RPM_NETDATA_PACKAGE_REQUIRES'])
                self.assertIn('netdata-plugin-dem', metadata['CPACK_DEBIAN_NETDATA_PACKAGE_RECOMMENDS'])
                self.assertEqual('netdata-plugin-dem = 2.10.0' in
                                 metadata['CPACK_RPM_NETDATA_PACKAGE_RECOMMENDS'], weak)
                self.assertEqual(metadata['CPACK_DEBIAN_PLUGIN-DEM_PACKAGE_SUGGESTS'], 'netdata-dem-runtime')
                self.assertEqual(metadata['CPACK_RPM_PLUGIN-DEM_PACKAGE_SUGGESTS'],
                                 'netdata-dem-runtime' if weak else '')

    def test_disabled_dem_adds_no_relationships(self):
        with tempfile.TemporaryDirectory() as temp:
            metadata = self.configure(Path(temp), 'debian', '12', enabled=False)
            self.assertTrue(all('netdata-plugin-dem' not in value and 'netdata-dem-runtime' not in value
                                for value in metadata.values()))


if __name__ == '__main__':
    unittest.main()

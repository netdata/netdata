# SPDX-License-Identifier: GPL-3.0-or-later
"""Exercise the runtime component through the Agent's real CPack configuration."""
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

SOURCE = Path(__file__).resolve().parents[1]
REPO = SOURCE.parents[2]
RELEASE_INFO = 'version=v2.12.0\nchannel=stable\nsource=' + 'a' * 40 + '\n'


def configure_agent(root, fmt='DEB', native_version='2.12.0', with_payload=True, paired=True,
                    kind=None, plugin=True, architecture='amd64'):
    source, build, payload = root / 'source', root / 'build', root / 'payload'
    (source / 'packaging').mkdir(parents=True)
    payload.mkdir(exist_ok=True)
    for name in ('LICENSE', 'README.md'):
        (source / name).symlink_to(REPO / name)
    (source / 'packaging/release-info').write_text(RELEASE_INFO)
    (payload / 'release-info').write_text(RELEASE_INFO if paired else RELEASE_INFO.replace('stable', 'nightly'))
    (payload / 'native.json').write_text('{}\n')
    (source / 'fixture').write_text('Agent package fixture\n')
    release = root / 'os-release'
    release.write_text('ID=' + ('debian\nVERSION_ID=11\n' if fmt == 'DEB' else 'rocky\nVERSION_ID=9\n'))
    (source / 'CMakeLists.txt').write_text('''
cmake_minimum_required(VERSION 3.16)
project(netdata VERSION 2.12.0 LANGUAGES NONE)
list(APPEND CMAKE_MODULE_PATH "${REPO}/packaging/cmake/Modules")
set(OS_LINUX TRUE)
set(PKG_FILES_PATH "${REPO}/packaging/cmake/pkg-files")
set(PLUGINS_DIR /usr/libexec/netdata/plugins.d)
set(netdata_group_POST netdata)
include(NetdataUtil)
install(FILES fixture DESTINATION usr/sbin COMPONENT netdata)
install(FILES fixture DESTINATION usr/libexec/netdata/plugins.d COMPONENT plugin-dem)
include(Packaging)
''')
    command = ['cmake', '-S', str(source), '-B', str(build), '-DREPO=' + str(REPO),
               '-DOS_RELEASE_PATH=' + str(release), '-DNETDATA_PACKAGE_VERSION=' + native_version,
               '-DCPACK_DEBIAN_PACKAGE_ARCHITECTURE=' + architecture,
               '-DNETDATA_PACKAGE_KIND=' + (kind or fmt.lower()),
               '-DENABLE_PLUGIN_DEM=' + ('ON' if plugin else 'OFF')]
    if with_payload:
        command.append('-DDEM_RUNTIME_PAYLOAD_DIR=' + str(payload))
    result = subprocess.run(command, capture_output=True, text=True)
    return result, build, payload


def read_config(root, config, keys):
    output, script = root / 'metadata', root / 'inspect.cmake'
    script.write_text('include("' + str(config) + '")\n' +
                      'file(WRITE "' + str(output) + '" "' +
                      ''.join(key + '=${' + key + '}\\n' for key in keys) + '")\n')
    result = subprocess.run(['cmake', '-P', str(script)], capture_output=True, text=True)
    if result.returncode:
        raise AssertionError(result.stdout + result.stderr)
    return dict(line.split('=', 1) for line in output.read_text().splitlines())


@unittest.skipUnless(shutil.which('cmake'), 'requires CMake')
class RecipeTests(unittest.TestCase):
    def test_shared_versions_and_separate_epoch_dependency_edges(self):
        for fmt in ('DEB', 'RPM'):
            for version in ('2.12.0', '2.12.0.9.nightly', '2.12.0~rc1'):
                with self.subTest(fmt=fmt, version=version), tempfile.TemporaryDirectory() as temp:
                    root = Path(temp)
                    result, build, _ = configure_agent(root, fmt=fmt, native_version=version)
                    self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                    keys = ['CPACK_COMPONENTS_ALL', 'CPACK_PACKAGE_VERSION', 'CPACK_RPM_PACKAGE_VERSION',
                            'CPACK_RPM_PACKAGE_RELEASE', 'CPACK_RPM_PACKAGE_EPOCH', 'CPACK_DEBIAN_PACKAGE_EPOCH',
                            'CPACK_DEBIAN_DEM-RUNTIME_PACKAGE_DEPENDS',
                            'CPACK_DEBIAN_DEM-RUNTIME_PACKAGE_PREDEPENDS',
                            'CPACK_DEBIAN_DEM-RUNTIME_FILE_NAME',
                            'CPACK_RPM_DEM-RUNTIME_PACKAGE_REQUIRES',
                            'CPACK_RPM_DEM-RUNTIME_PACKAGE_REQUIRES_PRE',
                            'CPACK_RPM_DEM-RUNTIME_PACKAGE_REQUIRES_PREUN']
                    ordinary = read_config(root, build / 'CPackConfig.cmake', keys)
                    runtime = read_config(root, build / 'CPackDEMRuntimeConfig.cmake', keys)
                    self.assertNotIn('dem-runtime', ordinary['CPACK_COMPONENTS_ALL'].split(';'))
                    self.assertEqual(runtime['CPACK_COMPONENTS_ALL'], 'dem-runtime')
                    self.assertEqual(ordinary['CPACK_PACKAGE_VERSION'], version)
                    self.assertEqual(runtime['CPACK_PACKAGE_VERSION'], version)
                    self.assertEqual(runtime['CPACK_RPM_PACKAGE_RELEASE'], ordinary['CPACK_RPM_PACKAGE_RELEASE'])
                    for key in ('CPACK_RPM_PACKAGE_EPOCH', 'CPACK_DEBIAN_PACKAGE_EPOCH'):
                        self.assertEqual(ordinary[key], '')
                        self.assertEqual(runtime[key], '1')
                    self.assertEqual(runtime['CPACK_DEBIAN_DEM-RUNTIME_PACKAGE_DEPENDS'],
                                     'netdata-plugin-dem (= ' + version + ')')
                    if fmt == 'DEB':
                        self.assertEqual(runtime['CPACK_DEBIAN_DEM-RUNTIME_FILE_NAME'],
                                         'netdata-dem-runtime_' + version + '_amd64.deb')
                    self.assertIn('netdata-plugin-dem = ' + version, runtime['CPACK_RPM_DEM-RUNTIME_PACKAGE_REQUIRES'])
                    for key in ('CPACK_DEBIAN_DEM-RUNTIME_PACKAGE_PREDEPENDS',
                                'CPACK_RPM_DEM-RUNTIME_PACKAGE_REQUIRES_PRE',
                                'CPACK_RPM_DEM-RUNTIME_PACKAGE_REQUIRES_PREUN'):
                        self.assertIn('netdata-dem-runtime-api', runtime[key])
                        self.assertNotIn('netdata-plugin-dem', runtime[key])
                    wrapper = (build / 'CPackDEMRuntimeConfig.cmake').read_text()
                    self.assertIn('Requires(posttrans): netdata-plugin-dem = ' + version, wrapper)
                    for family, hooks in [('deb', ('preinst', 'postinst', 'prerm', 'postrm')),
                                          ('rpm', ('pre', 'preun', 'posttrans', 'postun'))]:
                        for hook in hooks:
                            script = build / 'packaging/dem-runtime/hooks' / family / hook
                            checked = subprocess.run(['sh', '-n', str(script)], capture_output=True, text=True)
                            self.assertEqual(checked.returncode, 0, checked.stderr)
                            self.assertNotIn('@NATIVE_', script.read_text())

    def test_component_is_excluded_from_normal_install(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            result, build, _ = configure_agent(root)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            target = root / 'installed'
            installed = subprocess.run(['cmake', '--install', str(build), '--prefix', str(target)],
                                       capture_output=True, text=True)
            self.assertEqual(installed.returncode, 0, installed.stdout + installed.stderr)
            self.assertTrue((target / 'usr/sbin/fixture').exists())
            self.assertFalse((target / 'usr/lib/netdata/dem-runtime').exists())
            self.assertFalse((target / 'etc/apparmor.d/netdata-dem-browser').exists())
            component = subprocess.run(['cmake', '--install', str(build), '--prefix', str(target),
                                        '--component', 'dem-runtime'], capture_output=True, text=True)
            self.assertEqual(component.returncode, 0, component.stdout + component.stderr)
            self.assertTrue((target / 'usr/lib/netdata/dem-runtime/native.json').exists())

    def test_absent_input_has_no_runtime_component(self):
        with tempfile.TemporaryDirectory() as temporary:
            result, build, _ = configure_agent(Path(temporary), with_payload=False)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertFalse((build / 'CPackDEMRuntimeConfig.cmake').exists())
            self.assertNotIn('usr/lib/netdata/dem-runtime', (build / 'cmake_install.cmake').read_text())

    def test_unpaired_or_non_native_input_is_refused(self):
        for kwargs in ({'paired': False}, {'kind': 'bundle'}, {'plugin': False}):
            with self.subTest(kwargs=kwargs), tempfile.TemporaryDirectory() as temporary:
                result, _, _ = configure_agent(Path(temporary), **kwargs)
                self.assertNotEqual(result.returncode, 0)

    def test_generic_container_rpm_selection_excludes_optional_runtime_without_find(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            for name in ('netdata-2.12.0-1.x86_64.rpm', 'netdata-plugin-dem-2.12.0-1.x86_64.rpm',
                         'netdata-dem-runtime-2.12.0-1.x86_64.rpm'):
                (root / name).touch()
            script = (REPO / '.github/scripts/pkg-test.sh').read_text()
            prefix = script.split('install_debian_like()', 1)[0].replace('/netdata/artifacts', str(root))
            (root / 'netdata-directory.rpm').mkdir()
            # Minimal RPM images do not necessarily provide find before installation.
            probe = prefix + '\nfind() { return 127; }; agent_rpms'
            selected = subprocess.run(['sh', '-c', probe], capture_output=True, text=True)
            self.assertEqual(selected.returncode, 0, selected.stderr)
            self.assertEqual({Path(path).name for path in selected.stdout.splitlines()},
                             {'netdata-2.12.0-1.x86_64.rpm', 'netdata-plugin-dem-2.12.0-1.x86_64.rpm'})


if __name__ == '__main__':
    unittest.main()

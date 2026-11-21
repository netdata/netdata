# SPDX-License-Identifier: GPL-3.0-or-later
"""Build and inspect real package bytes; no package installation or service changes."""
import json
import os
from pathlib import Path
import platform
import shutil
import stat
import subprocess
import tempfile
import unittest

from test_recipe import configure_agent

SOURCE = Path(__file__).resolve().parents[1]


@unittest.skipUnless(platform.system() == 'Linux' and shutil.which('cmake') and shutil.which('cpack'),
                     'requires Linux CMake/CPack')
class PackageTests(unittest.TestCase):
    def run_command(self, *args):
        result = subprocess.run(list(map(str, args)), text=True, capture_output=True, umask=0o002)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        return result.stdout

    def extract_package(self, package, destination, fmt):
        destination.mkdir()
        if fmt == 'DEB':
            self.run_command('dpkg-deb', '--extract', package, destination)
        else:
            with tempfile.TemporaryFile() as archive:
                result = subprocess.run(['rpm2cpio', str(package)], stdout=archive, stderr=subprocess.PIPE)
                self.assertEqual(result.returncode, 0, result.stderr)
                archive.seek(0)
                result = subprocess.run(['cpio', '-idm', '--quiet'], cwd=destination, stdin=archive,
                                        stdout=subprocess.PIPE, stderr=subprocess.PIPE)
                self.assertEqual(result.returncode, 0, result.stderr)

    def check_package(self, fmt):
        tools = ('dpkg-deb',) if fmt == 'DEB' else ('rpmbuild', 'rpm', 'rpm2cpio', 'cpio')
        if any(not shutil.which(tool) for tool in tools):
            self.skipTest('requires ' + ', '.join(tools))
        arch = {'aarch64': 'arm64', 'x86_64': 'amd64'}.get(platform.machine())
        if arch is None:
            self.skipTest('requires a supported native architecture')
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            configured, build, payload = configure_agent(root, fmt=fmt, architecture=arch)
            self.assertEqual(configured.returncode, 0, configured.stdout + configured.stderr)
            extracted = root / 'extracted'
            (payload / 'lib').mkdir(parents=True)
            # Match stage-native permissions; only CMake-created ancestors inherit umask.
            payload.chmod(0o755)
            (payload / 'lib').chmod(0o755)
            shutil.copyfile('/bin/true', payload / 'node')
            (payload / 'node').chmod(0o755)
            (payload / 'lib/loader').write_bytes(b'private library fixture')
            (payload / 'lib/loader').chmod(0o644)
            (payload / 'lib/loader-link').symlink_to('loader')
            (payload / 'native.json').write_text(json.dumps({
                'format': 1, 'path': '/usr/lib/netdata/dem-runtime',
                'manifest': {'platform': 'linux/' + arch, 'version': 'v2.12.0'}}))
            (payload / 'native.json').chmod(0o644)
            ordinary_options = []
            if fmt == 'RPM':
                # This fixture stages two marker files rather than Agent binaries.
                ordinary_options = ['-D', 'CPACK_RPM_NETDATA_USER_FILELIST=',
                                    '-D', 'CPACK_RPM_PLUGIN-DEM_USER_FILELIST=']
            self.run_command('cpack', '-G', fmt, '--config', build / 'CPackConfig.cmake', *ordinary_options)
            self.assertFalse(list((build / 'packages').glob('netdata-dem-runtime*')))
            self.run_command('cpack', '-G', fmt, '--config', build / 'CPackDEMRuntimeConfig.cmake')
            packages = list((build / 'packages').glob('netdata-dem-runtime*.' + fmt.lower()))
            self.assertEqual(len(packages), 1)
            package = packages[0]
            self.extract_package(package, extracted, fmt)
            if fmt == 'DEB':
                self.assertEqual(package.name, 'netdata-dem-runtime_2.12.0_' + arch + '.deb')
                metadata = self.run_command('dpkg-deb', '--field', package, 'Pre-Depends')
                self.assertEqual(self.run_command('dpkg-deb', '--field', package, 'Architecture').strip(), arch)
                self.assertIn('netdata-dem-runtime-api (= 1)', metadata)
                self.assertEqual(self.run_command('dpkg-deb', '--field', package, 'Version').strip(), '1:2.12.0')
                self.assertIn('netdata-plugin-dem (= 2.12.0)',
                              self.run_command('dpkg-deb', '--field', package, 'Depends'))
                agent = next(path for path in (build / 'packages').glob('netdata_*.deb'))
                self.assertEqual(self.run_command('dpkg-deb', '--field', agent, 'Version').strip(), '2.12.0')
                plugin = next((build / 'packages').glob('netdata-plugin-dem_*.deb'))
                self.assertIn('netdata-ip-intel-data (= 2.12.0)',
                              self.run_command('dpkg-deb', '--field', plugin, 'Depends'))
                self.assertEqual(self.run_command('dpkg-deb', '--field', plugin, 'Breaks').strip(),
                                 'netdata-dem-runtime (<< 1:0)')
                control = root / 'control'
                self.run_command('dpkg-deb', '--control', package, control)
                for hook in ('preinst', 'postinst', 'prerm', 'postrm'):
                    self.assertEqual((control / hook).stat().st_mode & 0o777, 0o755)
                self.assertIn('/etc/apparmor.d/netdata-dem-browser', (control/'conffiles').read_text().splitlines())
                listing = self.run_command('dpkg-deb', '--contents', package)
                self.assertNotIn('/var/lib/netdata-dem-runtime', listing)
                self.assertTrue(all(' root/root ' in line for line in listing.splitlines()))
                directories = [line for line in listing.splitlines() if line.startswith('d')]
                self.assertTrue(directories)
                self.assertTrue(all(line.split()[0] == 'drwxr-xr-x' for line in directories),
                                '\n'.join(directories))
            else:
                dependencies = self.run_command('rpm', '-qp', '--requires', package)
                self.assertEqual(self.run_command('rpm', '-qp', '--qf', '%{ARCH}', package),
                                 {'amd64': 'x86_64', 'arm64': 'aarch64'}[arch])
                self.assertIn('netdata-dem-runtime-api = 1', dependencies)
                self.assertIn('netdata-plugin-dem = 2.12.0', dependencies)
                flags = self.run_command('rpm', '-qp', '--qf', '[%{REQUIRENAME} %{REQUIREFLAGS}\n]', package)
                requirements = [line.rsplit(' ', 1) for line in flags.splitlines()]
                helper_flags = [int(value) for name, value in requirements if name == 'netdata-dem-runtime-api']
                target_flags = [int(value) for name, value in requirements if name == 'netdata-plugin-dem']
                # RPM's script-pre, script-preun and script-posttrans sense bits.
                self.assertTrue(any(value & (1 << 9) for value in helper_flags))
                self.assertTrue(any(value & (1 << 11) for value in helper_flags))
                self.assertTrue(any(value & (1 << 5) for value in target_flags))
                self.assertEqual(self.run_command('rpm', '-qp', '--qf', '%{EPOCH}:%{VERSION}', package), '1:2.12.0')
                agent = next(path for path in (build / 'packages').glob('netdata-2*.rpm'))
                self.assertEqual(self.run_command('rpm', '-qp', '--qf', '%{EPOCH}', agent), '(none)')
                plugin = next((build / 'packages').glob('netdata-plugin-dem-*.rpm'))
                self.assertIn('netdata-ip-intel-data = 2.12.0',
                              self.run_command('rpm', '-qp', '--requires', plugin).splitlines())
                self.assertIn('netdata-dem-runtime < 1:0',
                              self.run_command('rpm', '-qp', '--conflicts', plugin).splitlines())
                ownership = self.run_command('rpm', '-qp', '--qf', '[%{FILEUSERNAME}:%{FILEGROUPNAME}\n]', package)
                self.assertEqual(set(ownership.splitlines()), {'root:root'})
                modes = self.run_command('rpm', '-qp', '--qf', '[%{FILEMODES:octal}\n]', package)
                directories = [int(mode, 8) for mode in modes.splitlines() if stat.S_ISDIR(int(mode, 8))]
                self.assertTrue(directories)
                self.assertTrue(all(mode & 0o777 == 0o755 for mode in directories), directories)
                flags = self.run_command('rpm', '-qp', '--qf', '[%{FILENAMES} %{FILEFLAGS:fflags}\n]', package)
                policy_flags = next(line.split()[-1] for line in flags.splitlines()
                                    if line.startswith('/etc/apparmor.d/netdata-dem-browser '))
                self.assertIn('c', policy_flags)
                self.assertIn('n', policy_flags)
                listing = self.run_command('rpm', '-qpl', package)
                self.assertNotIn('/var/lib/netdata-dem-runtime', listing)
            stock_path = Path('usr/share/netdata/topology-ip-intel')
            self.assertFalse((extracted / stock_path).exists())
            data_packages = list((build / 'packages').glob('netdata-ip-intel-data*.' + fmt.lower()))
            self.assertEqual(len(data_packages), 1)
            data_files = root / 'data-files'
            self.extract_package(data_packages[0], data_files, fmt)
            self.assertEqual({path.name for path in (data_files / stock_path).iterdir()},
                             {path.name for path in (root / 'stock').iterdir()})
            for path in (root / 'stock').iterdir():
                self.assertEqual((data_files / stock_path / path.name).read_bytes(), path.read_bytes())
            for consumer in (agent, plugin):
                consumer_files = root / consumer.name
                self.extract_package(consumer, consumer_files, fmt)
                self.assertFalse((consumer_files / stock_path).exists())
            policy = extracted / 'etc/apparmor.d/netdata-dem-browser'
            self.assertEqual(policy.read_bytes(), (SOURCE/'netdata-dem-browser').read_bytes())
            self.assertEqual(policy.stat().st_mode & 0o777, 0o644)
            installed = extracted / 'usr/lib/netdata/dem-runtime'
            for path in payload.rglob('*'):
                destination = installed / path.relative_to(payload)
                if path.is_symlink():
                    self.assertEqual(os.readlink(destination), os.readlink(path))
                elif path.is_file():
                    self.assertEqual(destination.read_bytes(), path.read_bytes())
                    self.assertEqual(destination.stat().st_mode & 0o777, path.stat().st_mode & 0o777)

    def test_deb_preserves_fixed_payload(self):
        self.check_package('DEB')

    @unittest.skipUnless(shutil.which('dpkg-deb') and shutil.which('dpkg'), 'requires Debian package tools')
    def test_deb_filenames_omit_epoch_but_control_preserves_it(self):
        architecture = self.run_command('dpkg', '--print-architecture').strip()
        for version in ('2.12.0', '2.12.0.9.nightly', '2.12.0~rc1'):
            with self.subTest(version=version), tempfile.TemporaryDirectory() as temporary:
                root = Path(temporary)
                configured, build, _ = configure_agent(root, native_version=version,
                                                       architecture=architecture)
                self.assertEqual(configured.returncode, 0, configured.stdout + configured.stderr)
                self.run_command('cpack', '-G', 'DEB', '--config', build / 'CPackDEMRuntimeConfig.cmake')
                packages = list((build / 'packages').glob('netdata-dem-runtime*.deb'))
                self.assertEqual([path.name for path in packages],
                                 ['netdata-dem-runtime_' + version + '_' + architecture + '.deb'])
                self.assertEqual(self.run_command('dpkg-deb', '--field', packages[0], 'Version').strip(),
                                 '1:' + version)

    def test_rpm_preserves_fixed_payload(self):
        self.check_package('RPM')


if __name__ == '__main__':
    unittest.main()

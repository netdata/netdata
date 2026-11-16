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

SOURCE = Path(__file__).resolve().parents[1]


@unittest.skipUnless(platform.system() == 'Linux' and shutil.which('cmake') and shutil.which('cpack'),
                     'requires Linux CMake/CPack')
class PackageTests(unittest.TestCase):
    def run_command(self, *args):
        result = subprocess.run(list(map(str, args)), text=True, capture_output=True, umask=0o002)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        return result.stdout

    def check_package(self, fmt):
        tools = ('dpkg-deb',) if fmt == 'DEB' else ('rpmbuild', 'rpm', 'rpm2cpio', 'cpio')
        if any(not shutil.which(tool) for tool in tools):
            self.skipTest('requires ' + ', '.join(tools))
        arch = {'aarch64': 'arm64', 'x86_64': 'amd64'}.get(platform.machine())
        if arch is None:
            self.skipTest('requires a supported native architecture')
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            payload, build, extracted = root / 'payload', root / 'build', root / 'extracted'
            (payload / 'lib').mkdir(parents=True)
            extracted.mkdir()
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
                'manifest': {'platform': 'linux/' + arch, 'version': '1.2.3'}}))
            (payload / 'native.json').chmod(0o644)
            self.run_command('cmake', '-S', SOURCE, '-B', build,
                             '-DDEM_PAYLOAD_DIR=' + str(payload), '-DDEM_ARCH=' + arch,
                             '-DDEM_VERSION=1.2.3', '-DDEM_RELEASE=1', '-DDEM_PACKAGE_FORMAT=' + fmt)
            self.run_command('cpack', '--config', build / 'CPackConfig.cmake')
            packages = list((build / 'packages').glob('*.' + fmt.lower()))
            self.assertEqual(len(packages), 1)
            package = packages[0]
            if fmt == 'DEB':
                self.run_command('dpkg-deb', '--extract', package, extracted)
                metadata = self.run_command('dpkg-deb', '--field', package, 'Pre-Depends')
                self.assertIn('netdata-dem-runtime-api (= 1)', metadata)
                control = root / 'control'
                self.run_command('dpkg-deb', '--control', package, control)
                self.assertIn('/etc/apparmor.d/netdata-dem-browser', (control/'conffiles').read_text().splitlines())
                listing = self.run_command('dpkg-deb', '--contents', package)
                self.assertNotIn('/var/lib/netdata-dem-runtime', listing)
                self.assertTrue(all(' root/root ' in line for line in listing.splitlines()))
                directories = [line for line in listing.splitlines() if line.startswith('d')]
                self.assertTrue(directories)
                self.assertTrue(all(line.split()[0] == 'drwxr-xr-x' for line in directories),
                                '\n'.join(directories))
            else:
                archive = root / 'payload.cpio'
                with archive.open('wb') as output:
                    result = subprocess.run(['rpm2cpio', str(package)], stdout=output, stderr=subprocess.PIPE)
                self.assertEqual(result.returncode, 0, result.stderr)
                with archive.open('rb') as source:
                    result = subprocess.run(['cpio', '-idm', '--quiet'], cwd=extracted, stdin=source,
                                            stdout=subprocess.PIPE, stderr=subprocess.PIPE)
                self.assertEqual(result.returncode, 0, result.stderr)
                dependencies = self.run_command('rpm', '-qp', '--requires', package)
                self.assertIn('netdata-dem-runtime-api = 1', dependencies)
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

    def test_rpm_preserves_fixed_payload(self):
        self.check_package('RPM')


if __name__ == '__main__':
    unittest.main()

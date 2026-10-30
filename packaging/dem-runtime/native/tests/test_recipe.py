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
    def configure(self, root, arch='arm64', fmt='DEB', version='1.2.3'):
        payload = root / 'payload'
        payload.mkdir(exist_ok=True)
        (payload/'native.json').write_text(json.dumps({
            'format': 1, 'path': '/usr/lib/netdata/dem-runtime',
            'manifest': {'platform': 'linux/arm64', 'version': '1.2.3'}}))
        build = root / 'build'
        result = subprocess.run(['cmake', '-S', str(SOURCE), '-B', str(build),
                                 '-DCMAKE_SYSTEM_NAME=Linux', '-DDEM_PAYLOAD_DIR='+str(payload),
                                 '-DDEM_ARCH='+arch, '-DDEM_VERSION='+version,
                                 '-DDEM_RELEASE=1', '-DDEM_PACKAGE_FORMAT='+fmt],
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

    def test_requested_identity_must_match_staging(self):
        for kwargs in ({'arch':'amd64'},{'version':'9.9'},{'arch':'s390x'},{'version':'1;bad'}):
            with self.subTest(kwargs=kwargs), tempfile.TemporaryDirectory() as temp:
                result,_ = self.configure(Path(temp),**kwargs)
                self.assertNotEqual(result.returncode,0)


if __name__ == '__main__':
    unittest.main()

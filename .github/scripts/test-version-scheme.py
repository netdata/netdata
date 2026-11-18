#!/usr/bin/env python3
"""Check package, project (MSI) and updater orderings for every version form."""

from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

REPO = Path(__file__).resolve().parents[2]
MODULE = REPO / 'packaging/cmake/Modules/NetdataVersion.cmake'
UPDATER = REPO / 'packaging/installer/netdata-updater.sh'

# version: (package version, project/MSI version)
FORMS = {
    'v2.14.0': ('2.14.0', '2.14.100.0'),
    'v2.14.1': ('2.14.1', '2.14.101.0'),
    'v2.14.0-rc01': ('2.14.0~rc01', '2.14.1.0'),
    'v2.14.0-rc02': ('2.14.0~rc02', '2.14.2.0'),
    'v2.14.0-rc10': ('2.14.0~rc10', '2.14.10.0'),
    'v2.14.0-rc01-3-g3340f53a85': ('2.14.0~rc01.git.3.g3340f53a85', '2.14.1.3'),
    'v2.14-941-nightly': ('2.14.0~nightly.941', '2.14.0.941'),
    'v2.15-0-nightly': ('2.15.0~nightly.0', '2.15.0.0'),
    'v2.15-37-nightly': ('2.15.0~nightly.37', '2.15.0.37'),
    'v2.13.0-29-post-nightly': ('2.13.0.29.post', '2.13.100.29'),
    'v2.13.0-29-nightly': ('2.13.0.29.post', '2.13.100.29'),
    'v2.15-617-g3340f53a85': ('2.15.0~git.617.g3340f53a85', '2.15.0.617'),
    'v2.15': ('2.15.0~git.0', '2.15.0.0'),
    'v2.14.0-3-g3340f53a85': ('2.14.0.git.3.g3340f53a85', '2.14.100.3'),
}

# Ascending release order shared by every consumer.
ORDER = ['v2.13.0', 'v2.13.0-29-post-nightly', 'v2.14-29-nightly', 'v2.14-941-nightly', 'v2.14.0-rc01',
         'v2.14.0-rc02', 'v2.14.0-rc10', 'v2.14.0', 'v2.14.1', 'v2.15-37-nightly', 'v2.15.0-rc01', 'v2.15.0']


@unittest.skipUnless(shutil.which('cmake'), 'requires CMake')
class CMakeVersionTest(unittest.TestCase):
    def parse(self, version):
        with tempfile.TemporaryDirectory() as directory:
            script = Path(directory) / 'parse.cmake'
            script.write_text(f'include("{MODULE}")\nnetdata_parse_version("{version}")\n'
                              'message("${NETDATA_PACKAGE_VERSION} ${NETDATA_PROJECT_VERSION}")\n')
            result = subprocess.run(['cmake', '-P', str(script)], capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        return tuple(result.stderr.split())

    def test_forms(self):
        for version, expected in FORMS.items():
            with self.subTest(version=version):
                self.assertEqual(self.parse(version), expected)

    def test_rejects_unknown(self):
        with tempfile.TemporaryDirectory() as directory:
            script = Path(directory) / 'parse.cmake'
            script.write_text(f'include("{MODULE}")\nnetdata_parse_version("v2.14.0-beta1")\n')
            result = subprocess.run(['cmake', '-P', str(script)], capture_output=True, text=True)
        self.assertNotEqual(result.returncode, 0)

    @unittest.skipUnless(shutil.which('dpkg'), 'requires dpkg')
    def test_deb_order(self):
        versions = [self.parse(version)[0] for version in ORDER]
        for older, newer in zip(versions, versions[1:]):
            with self.subTest(older=older, newer=newer):
                subprocess.run(['dpkg', '--compare-versions', older, 'lt', newer], check=True)

    @unittest.skipUnless(shutil.which('rpm'), 'requires rpm')
    def test_rpm_order(self):
        versions = [self.parse(version)[0] for version in ORDER]
        for older, newer in zip(versions, versions[1:]):
            with self.subTest(older=older, newer=newer):
                result = subprocess.run(['rpm', '--eval', f'%{{lua: print(rpm.vercmp("{older}", "{newer}"))}}'],
                                        capture_output=True, text=True, check=True)
                self.assertEqual(result.stdout.strip(), '-1')

    def test_project_versions_have_four_fields(self):
        # Win32 assemblyIdentity versions (packaging/windows/resources/*.manifest.in) require four fields.
        for version in FORMS:
            with self.subTest(version=version):
                self.assertEqual(len(self.parse(version)[1].split('.')), 4)

    def test_msi_order(self):
        # MSI compares only major.minor.build; nightlies share the field with the version they follow.
        def key(version):
            return tuple(int(part) for part in self.parse(version)[1].split('.')[:3])
        releases = [version for version in ORDER if 'nightly' not in version]
        for older, newer in zip(releases, releases[1:]):
            with self.subTest(older=older, newer=newer):
                self.assertLess(key(older), key(newer))


class UpdaterOrderTest(unittest.TestCase):
    def test_order(self):
        script = (f'. <(sed -n "/^parse_version() {{/,/^}}/p" {UPDATER})\n'
                  'warning() { echo "$@" >&2; exit 1; }\n'
                  'for v in "$@"; do parse_version "$v"; echo; done')
        result = subprocess.run(['bash', '-c', script, 'order', *ORDER, 'v2.14.0-rc08', 'v2.14.0-rc09'],
                                capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        keys = [int(line) for line in result.stdout.split()]
        self.assertEqual(keys[:len(ORDER)], sorted(keys[:len(ORDER)]))
        self.assertEqual(len(set(keys[:len(ORDER)])), len(ORDER))
        self.assertLess(keys[-2], keys[-1])


if __name__ == '__main__':
    unittest.main()

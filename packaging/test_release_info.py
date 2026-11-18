# SPDX-License-Identifier: GPL-3.0-or-later
"""Official build identity stays separate from existing Agent version encoding."""
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

import release_info


ROOT = Path(__file__).resolve().parent.parent
SOURCE = 'a' * 40


class ReleaseInfoTests(unittest.TestCase):
    def test_strict_reader(self):
        valid = f'version=v2.12.0-9-nightly\nchannel=nightly\nsource={SOURCE}\n'
        cases = {
            'stable': valid.replace('v2.12.0-9-nightly', 'v2.12.0').replace('nightly\n', 'stable\n'),
            'nightly': valid,
            'devel': valid.replace('channel=nightly', 'channel=devel'),
            'single character version': valid.replace('v2.12.0-9-nightly', '1'),
            'maximum version': valid.replace('v2.12.0-9-nightly', 'v' * 128),
        }
        invalid = {
            'missing': valid.replace('channel=nightly\n', ''),
            'duplicate': valid.replace('channel=nightly', 'version=nightly'),
            'unknown': valid.replace('channel=', 'extra='),
            'empty': valid.replace('channel=nightly', 'channel='),
            'whitespace': valid.replace('version=', 'version= '),
            'path': valid.replace('v2.12.0-9-nightly', '../release'),
            'overlong': valid.replace('v2.12.0-9-nightly', 'v' * 129),
            'channel': valid.replace('channel=nightly', 'channel=preview'),
            'short SHA': valid.replace(SOURCE, 'a' * 39),
            'uppercase SHA': valid.replace(SOURCE, 'A' * 40),
            'not hex': valid.replace(SOURCE, 'x' * 40),
            'blank line': valid + '\n',
            'unterminated': valid[:-1],
            'CRLF': valid.replace('\n', '\r\n'),
            'injection': valid.replace('v2.12.0-9-nightly', '${INJECTED}'),
            'semicolon': valid.replace('v2.12.0-9-nightly', 'v2;injected'),
            'reordered': 'channel=nightly\nversion=v2.12.0\nsource=' + SOURCE + '\n',
        }
        with tempfile.TemporaryDirectory() as temporary:
            path = Path(temporary) / 'release-info'
            for name, body in cases.items():
                with self.subTest(name=name):
                    path.write_text(body)
                    info = release_info.read_release_info(path)
                    self.assertEqual(info['source'], SOURCE)
            for name, body in invalid.items():
                with self.subTest(name=name):
                    path.write_text(body)
                    with self.assertRaises(ValueError):
                        release_info.read_release_info(path)

    @unittest.skipUnless(shutil.which('git'), 'requires Git')
    def test_stamp_checks_frozen_source_before_replacing_metadata(self):
        with tempfile.TemporaryDirectory(prefix='release identity ') as temporary:
            root = Path(temporary)
            (root / 'packaging').mkdir()
            (root / 'packaging/version').write_text('v2.12.0\n')

            def git(*args):
                return subprocess.run(['git', *args], cwd=root, check=True,
                                      capture_output=True, text=True).stdout.strip()
            git('init', '-q')
            git('-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.invalid',
                'commit', '--allow-empty', '-qm', 'Fixture')
            source = git('rev-parse', 'HEAD')
            expected = {'version': 'v2.12.0', 'channel': 'stable', 'source': source}
            self.assertEqual(release_info.stamp(root, 'stable', source), expected)
            path = root / 'packaging/release-info'
            self.assertEqual(release_info.read_release_info(path), expected)
            original = path.read_bytes()
            with self.assertRaisesRegex(ValueError, 'differs'):
                release_info.stamp(root, 'nightly', SOURCE)
            self.assertEqual(path.read_bytes(), original)
            expected = {'version': '0.' + source, 'channel': 'devel', 'source': source}
            self.assertEqual(release_info.stamp(root, 'devel', source), expected)
            self.assertEqual(release_info.read_release_info(path), expected)
            self.assertEqual((root / 'packaging/version').read_text(), 'v2.12.0\n')

    def _stamp_fixture(self, root):
        (root / 'packaging').mkdir()
        (root / 'packaging/version').write_text('v2.12.0\n')
        for args in (['init', '-q'],
                     ['-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.invalid',
                      'commit', '--allow-empty', '-qm', 'Fixture']):
            subprocess.run(['git', *args], cwd=root, check=True, capture_output=True)
        return subprocess.run(['git', 'rev-parse', 'HEAD'], cwd=root, check=True,
                              capture_output=True, text=True).stdout.strip()

    @unittest.skipUnless(shutil.which('git'), 'requires Git')
    def test_stamp_without_fchmod(self):
        # Windows CPython before 3.13 has no os.fchmod.
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            source = self._stamp_fixture(root)
            saved = os.fchmod
            del os.fchmod
            try:
                release_info.stamp(root, 'stable', source)
            finally:
                os.fchmod = saved
            path = root / 'packaging/release-info'
            self.assertEqual(release_info.read_release_info(path)['source'], source)
            if os.name == 'posix':
                self.assertEqual(path.stat().st_mode & 0o777, 0o644)

    @unittest.skipUnless(shutil.which('git'), 'requires Git')
    def test_stamp_writes_lf_under_windows_newline_translation(self):
        real = tempfile.NamedTemporaryFile

        def windows_text_mode(*args, **kwargs):
            # Text mode with the default newline writes CRLF on Windows.
            if 'b' not in kwargs.get('mode', 'w+b'):
                kwargs.setdefault('newline', '\r\n')
            return real(*args, **kwargs)

        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            source = self._stamp_fixture(root)
            with mock.patch.object(release_info.tempfile, 'NamedTemporaryFile', windows_text_mode):
                release_info.stamp(root, 'stable', source)
            path = root / 'packaging/release-info'
            self.assertEqual(path.read_bytes(),
                             f'version=v2.12.0\nchannel=stable\nsource={source}\n'.encode('ascii'))

    def test_cli_preserves_git_failure_status_and_diagnostics(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            (root / 'packaging').mkdir()
            (root / 'packaging/version').write_text('v2.12.0\n')
            git = root / 'git'
            git.write_text('#!/bin/sh\nprintf "%s\\n" "fixture Git failure" >&2\nexit 23\n')
            git.chmod(0o755)
            result = subprocess.run(
                [sys.executable, str(ROOT / 'packaging/release_info.py'), '--root', str(root),
                 '--channel', 'stable', '--source', SOURCE],
                env=dict(os.environ, PATH=str(root)), capture_output=True, text=True)
            self.assertEqual(result.returncode, 23)
            self.assertIn('status 23', result.stderr)
            self.assertIn('fixture Git failure', result.stderr)
            self.assertEqual(result.stdout, '')
            self.assertFalse((root / 'packaging/release-info').exists())


@unittest.skipUnless(shutil.which('cmake') and shutil.which('git'), 'requires CMake and Git')
class CMakeReleaseInfoTests(unittest.TestCase):
    def configure(self, body=None, explicit='', git_version=None):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary)
            (root / 'packaging').mkdir()
            (root / 'packaging/version').write_text('v2.12.0-9-nightly\n')
            if body is not None:
                (root / 'packaging/release-info').write_text(body)
            if git_version is not None:
                for arguments in [('init', '-q'),
                                  ('-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.invalid',
                                   'commit', '--allow-empty', '-qm', 'Fixture'),
                                  ('-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.invalid',
                                   'tag', '-a', git_version, '-m', 'Fixture')]:
                    subprocess.run(['git', *arguments], cwd=root, check=True, capture_output=True)
            script = root / 'inspect.cmake'
            script.write_text(
                'cmake_minimum_required(VERSION 3.16)\n'
                f'set(CMAKE_SOURCE_DIR "{root}")\n' + explicit + '\n'
                f'include("{ROOT}/packaging/cmake/Modules/NetdataVersion.cmake")\n'
                'netdata_version()\n'
                f'include("{ROOT}/packaging/cmake/Modules/NetdataGoTools.cmake")\n'
                f'file(WRITE "{root}/result" "${{NETDATA_VERSION_STRING}}\\n'
                '${NETDATA_PACKAGE_VERSION}\\n${NETDATA_RELEASE_VERSION}\\n'
                '${NETDATA_RELEASE_CHANNEL}\\n${NETDATA_RELEASE_REF}\\n${GO_LDFLAGS}\\n")\n')
            result = subprocess.run(['cmake', '-P', str(script)], cwd=root, capture_output=True, text=True)
            output = (root / 'result').read_text().splitlines() if (root / 'result').exists() else []
            return result, output

    def test_developer_build_has_no_official_identity(self):
        result, output = self.configure()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(output[:5], ['v2.12.0-9-nightly', '2.12.0.9.nightly', '', '', ''])
        self.assertIn('.ReleaseVersion=', output[5])

    def test_official_identity_does_not_change_display_or_native_version(self):
        for version, channel in [('v2.13.0', 'stable'), ('v2.13.0-20-nightly', 'nightly'), ('development', 'devel')]:
            with self.subTest(channel=channel):
                result, output = self.configure(f'version={version}\nchannel={channel}\nsource={SOURCE}\n')
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                self.assertEqual(output[:5], ['v2.12.0-9-nightly', '2.12.0.9.nightly', version, channel, SOURCE])
                self.assertIn('.ReleaseVersion=' + version, output[5])
                self.assertIn('.ReleaseChannel=' + channel, output[5])
                self.assertIn('.ReleaseRef=' + SOURCE, output[5])

    def test_git_display_version_is_preserved(self):
        result, output = self.configure(f'version=v2.13.0\nchannel=stable\nsource={SOURCE}\n',
                                        git_version='v2.10.0')
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(output[:5], ['v2.10.0', '2.10.0', 'v2.13.0', 'stable', SOURCE])

    def test_malformed_metadata_and_explicit_inputs_are_rejected(self):
        valid = f'version=v2.12.0\nchannel=stable\nsource={SOURCE}\n'
        for body in [valid + '\n', valid.replace('channel=stable', 'version=duplicate'),
                     valid.replace('channel=stable', 'unknown=stable'), valid.replace(SOURCE, 'a' * 39),
                     valid.replace('v2.12.0', '${INJECTED}'), valid.replace('v2.12.0', 'v2;injected'),
                     valid.replace('v2.12.0', 'v' * 129), valid.replace('\n', '\r\n'), valid[:-1],
                     'channel=stable\nversion=v2.12.0\nsource=' + SOURCE + '\n']:
            with self.subTest(body=body):
                result, _ = self.configure(body)
                self.assertNotEqual(result.returncode, 0)
        result, _ = self.configure(explicit='set(NETDATA_RELEASE_VERSION "v2.12.0")')
        self.assertNotEqual(result.returncode, 0)


if __name__ == '__main__':
    unittest.main()

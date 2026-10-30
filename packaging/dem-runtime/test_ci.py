# SPDX-License-Identifier: GPL-3.0-or-later
"""Repository submission coverage through the existing package uploader."""
from contextlib import redirect_stderr
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

import ci


def catalog():
    return {'arch_data': {'amd64': {'runner': 'next-amd64'}, 'arm64': {'runner': 'next-arm64'}},
            'include': [
                {'packages': {'type': 'deb', 'repo_distro': 'debian/new', 'arches': ['amd64', 'armhf']}},
                {'packages': {'type': 'rpm', 'repo_distro': 'el/new', 'arches': ['aarch64']}},
                {'packages': {'type': 'rpm', 'repo_distro': 'el/new', 'arches': ['aarch64']}},
                {'packages': {}}, {'distro': 'without-packages'}]}


class CITests(unittest.TestCase):
    def test_build_runners_follow_shared_catalog(self):
        self.assertEqual(ci.build_matrix(catalog()), {'include': [
            {'arch': 'amd64', 'runner': 'next-amd64'},
            {'arch': 'arm64', 'runner': 'next-arm64'}]})

    def test_targets_filter_architectures_and_deduplicate_destinations(self):
        self.assertEqual(ci.package_targets(catalog()), [
            ('debian/new', 'deb', 'amd64', 'amd64'), ('el/new', 'rpm', 'aarch64', 'arm64')])

    def fixture(self, root, exit_status=0):
        artifacts = root / 'artifacts'
        for arch, fmt in [('amd64', 'deb'), ('arm64', 'rpm')]:
            directory = artifacts / ('dem-runtime-' + arch)
            directory.mkdir(parents=True)
            (directory / ('netdata-dem-runtime.' + fmt)).write_text(arch)
        binary = root / 'bin'
        binary.mkdir()
        rsync = binary / 'rsync'
        rsync.write_text('''#!/usr/bin/env python3
import json, os, pathlib, sys
source = pathlib.Path(sys.argv[-2])
entry = {'destination': sys.argv[-1], 'source': str(source),
         'files': {str(p.relative_to(source)): p.read_text() for p in source.rglob('*') if p.is_file()}}
with open(os.environ['DEM_UPLOAD_TEST_LOG'], 'a') as stream:
    stream.write(json.dumps(entry) + '\\n')
sys.exit(int(os.environ['DEM_UPLOAD_TEST_EXIT']))
''')
        rsync.chmod(0o755)
        log = root / 'uploads.jsonl'
        return artifacts, log, {'PATH': str(binary) + os.pathsep + os.environ['PATH'],
                                'DEM_UPLOAD_TEST_LOG': str(log), 'DEM_UPLOAD_TEST_EXIT': str(exit_status)}

    def test_real_uploader_covers_both_hosts_all_channels_without_staging_leaks(self):
        with tempfile.TemporaryDirectory() as temporary:
            artifacts, log, env = self.fixture(Path(temporary))
            with patch.dict(os.environ, env), tempfile.TemporaryFile(mode='w+') as output, redirect_stderr(output):
                ci.upload_native(catalog(), artifacts)
            rows = [json.loads(line) for line in log.read_text().splitlines()]
            self.assertEqual(len(rows), 12)
            expected_destinations = {
                'netdatabot@' + host + ':/home/netdatabot/incoming/' + repo + '/'
                for host in ci.PACKAGE_HOSTS for repo in ci.PACKAGE_REPOSITORIES}
            self.assertEqual({row['destination'] for row in rows}, expected_destinations)
            self.assertFalse(any('repoconfig' in row['destination'] for row in rows))
            for destination in expected_destinations:
                sent = [row for row in rows if row['destination'] == destination]
                self.assertCountEqual([row['files'] for row in sent], [
                    {'debian/new/netdata-dem-runtime.deb': 'amd64'},
                    {'el/new/aarch64/netdata-dem-runtime.rpm': 'arm64'}])
            self.assertEqual(len({row['source'] for row in rows}), len(rows))
            self.assertTrue(all(not Path(row['source']).exists() for row in rows))

    def test_failed_upload_preserves_status_and_stops_remaining_targets(self):
        with tempfile.TemporaryDirectory() as temporary:
            artifacts, log, env = self.fixture(Path(temporary), exit_status=17)
            with patch.dict(os.environ, env), tempfile.TemporaryFile(mode='w+') as output, redirect_stderr(output):
                with self.assertRaises(subprocess.CalledProcessError) as failure:
                    ci.upload_native(catalog(), artifacts)
            self.assertEqual(failure.exception.returncode, 17)
            rows = [json.loads(line) for line in log.read_text().splitlines()]
            self.assertEqual(len(rows), 1)
            self.assertFalse(Path(rows[0]['source']).exists())

    def test_missing_package_fails_before_any_submission(self):
        with tempfile.TemporaryDirectory() as temporary:
            artifacts, log, env = self.fixture(Path(temporary))
            data = catalog()
            data['include'][0]['packages']['arches'].append('arm64')
            with patch.dict(os.environ, env), self.assertRaises(ValueError):
                ci.upload_native(data, artifacts)
            self.assertFalse(log.exists())


if __name__ == '__main__':
    unittest.main()

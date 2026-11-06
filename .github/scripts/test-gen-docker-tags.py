#!/usr/bin/env python3

import os
from pathlib import Path
import runpy
import tempfile
import unittest
from unittest.mock import MagicMock, patch
from urllib.error import HTTPError, URLError


SCRIPT = Path(__file__).with_name('gen-docker-tags.py')
REPOS = ('netdata/netdata', 'quay.io/netdata/netdata', 'ghcr.io/netdata/netdata')


class DockerTagsTest(unittest.TestCase):
    def generate(self, version, preview='v2.12.2\n', error=None):
        response = MagicMock()
        response.__enter__.return_value.read.return_value = preview.encode()
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / 'output'
            with patch.dict(os.environ, GITHUB_OUTPUT=str(output)), \
                 patch('sys.argv', [str(SCRIPT), 'workflow_dispatch', version]), \
                 patch('urllib.request.urlopen', return_value=response, side_effect=error) as fetch:
                runpy.run_path(str(SCRIPT), run_name='__main__')
            result = dict(line.split('=', 1) for line in output.read_text().splitlines())
        return result, fetch

    def test_stable_comparison(self):
        cases = (
            ('2.12.1', 'v2.12.2', False),
            ('2.12.2', 'v2.12.2\n', True),
            ('2.12.3', 'v2.12.2', True),
            ('2.9.9', 'v2.12.2', False),
            ('2.12.2', 'v2.9.9', True),
            ('2.99.99', 'v3.0.0', False),
            ('3.0.0', 'v2.99.99', True),
            ('2.12.2', 'v2.12.2-rc.1', True),
            ('2.12.1', 'v2.12.2-rc.1', False),
            ('2.12.2', '2.12.2+build.7', True),
        )
        for stable, preview, eligible in cases:
            with self.subTest(stable=stable, preview=preview):
                result, fetch = self.generate(stable, preview)
                fetch.assert_called_once_with(
                    'https://artifacts.netdata.cloud/preview/latest/Version', timeout=30,
                )
                major, minor, _ = stable.split('.')
                suffixes = [f'v{major}', f'v{major}.{minor}', f'v{stable}', 'stable']
                if eligible:
                    suffixes.append('preview')
                expected = [f'{repo}:{tag}' for repo in REPOS for tag in suffixes]
                self.assertEqual(result['tags'], ','.join(expected))
                for registry, repo in zip(('docker', 'quay', 'ghcr'), REPOS):
                    self.assertEqual(result[f'{registry}_tags'], ','.join(
                        f'{repo}:{tag}' for tag in suffixes
                    ))
                self.assertEqual(result['nightly'], '0')

    def test_build_metadata_hyphen_preserves_stable_eligibility(self):
        result, fetch = self.generate('2.12.2+build-7', 'v2.12.2')
        fetch.assert_called_once_with(
            'https://artifacts.netdata.cloud/preview/latest/Version', timeout=30,
        )
        for registry, repo in zip(('docker', 'quay', 'ghcr'), REPOS):
            self.assertIn(f'{repo}:preview', result[f'{registry}_tags'].split(','))

    def test_non_stable_does_not_fetch(self):
        for version, suffixes in (('', ['test']), ('nightly', ['edge', 'latest'])):
            with self.subTest(version=version):
                result, fetch = self.generate(version)
                fetch.assert_not_called()
                self.assertEqual(result['tags'], ','.join(
                    f'{repo}:{tag}' for repo in REPOS for tag in suffixes
                ))
                self.assertEqual(result['nightly'], '1' if version else '0')
                self.assertEqual(result['nightly_tag'], 'edge')

    def test_invalid_versions_fail(self):
        for preview in ('', 'garbage', 'v2.12', 'v02.12.2', 'v2.12.2-01',
                        'v2.12.2+', 'v2.12.2 trailing'):
            with self.subTest(preview=preview), self.assertRaises(ValueError):
                self.generate('2.12.2', preview)

    def test_prerelease_does_not_fetch(self):
        for version, suffixes in (
            ('2.12.2-rc1', ['v2.12.2-rc1']),
            ('2.12.2-rc.1', ['v2.12.2-rc.1']),
        ):
            with self.subTest(version=version):
                result, fetch = self.generate(version, error=URLError('unavailable'))
                fetch.assert_not_called()
                self.assertEqual(result['tags'], ','.join(
                    f'{repo}:{tag}' for repo in REPOS for tag in suffixes
                ))
                self.assertEqual(result['nightly'], '0')

    def test_invalid_explicit_versions_fail(self):
        for version in ('garbage-rc', '2.12-rc.1', '2.12.2-01',
                        '2.12.2-rc..1', '02.12.2-rc.1'):
            with self.subTest(version=version), self.assertRaises(ValueError):
                self.generate(version, error=URLError('unavailable'))

    def test_missing_preview_is_eligible(self):
        error = HTTPError('https://example.invalid/Version', 404, 'Not Found', None, None)
        result, _ = self.generate('2.12.2', error=error)
        for registry, repo in zip(('docker', 'quay', 'ghcr'), REPOS):
            self.assertIn(f'{repo}:preview', result[f'{registry}_tags'].split(','))

    def test_network_failure_fails(self):
        with self.assertRaises(URLError):
            self.generate('2.12.2', error=URLError('unavailable'))


if __name__ == '__main__':
    unittest.main()

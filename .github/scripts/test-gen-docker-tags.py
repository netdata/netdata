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


PREFIX = 'https://artifacts.netdata.cloud/'


class DockerTagsTest(unittest.TestCase):
    def generate(self, version, published=None, error=None):
        """Run the generator; PUBLISHED maps aliases such as 'stable/2.12' to their Version (missing = 404)."""
        published = {} if published is None else published

        def fetch(url, timeout):
            self.assertEqual(timeout, 30)
            if error is not None:
                raise error
            alias = url.removeprefix(PREFIX).removesuffix('/Version')
            if alias not in published:
                raise HTTPError(url, 404, 'Not Found', None, None)
            response = MagicMock()
            response.__enter__.return_value.read.return_value = published[alias].encode()
            return response

        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / 'output'
            with patch.dict(os.environ, GITHUB_OUTPUT=str(output)), \
                 patch('sys.argv', [str(SCRIPT), 'workflow_dispatch', version]), \
                 patch('urllib.request.urlopen', side_effect=fetch) as mock:
                runpy.run_path(str(SCRIPT), run_name='__main__')
            result = dict(line.split('=', 1) for line in output.read_text().splitlines())
        return result, mock

    def assert_tags(self, result, suffixes):
        self.assertEqual(result['tags'], ','.join(f'{repo}:{tag}' for repo in REPOS for tag in suffixes))
        for registry, repo in zip(('docker', 'quay', 'ghcr'), REPOS):
            self.assertEqual(result[f'{registry}_tags'], ','.join(f'{repo}:{tag}' for tag in suffixes))

    def test_stable_aliases_never_move_backwards(self):
        published = {'stable/latest': 'v2.14.0', 'stable/2': 'v2.14.0', 'stable/2.13': 'v2.13.4',
                     'stable/2.14': 'v2.14.0', 'preview/latest': 'v2.15.0-rc01'}
        cases = (
            # A patch for an older series only moves its own series alias.
            ('2.13.5', published, ['v2.13.5', 'latest', 'v2.13']),
            ('2.13.3', published, ['v2.13.3', 'latest']),
            ('2.14.1', published, ['v2.14.1', 'latest', 'v2', 'v2.14', 'stable']),
            ('2.15.0', published, ['v2.15.0', 'latest', 'v2', 'v2.15', 'stable', 'preview']),
            ('2.14.1', {}, ['v2.14.1', 'latest', 'v2', 'v2.14', 'stable', 'preview']),
            ('2.14.0', {'preview/latest': 'v2.14.0-rc03'},
             ['v2.14.0', 'latest', 'v2', 'v2.14', 'stable', 'preview']),
            ('2.14.0+build-7', {'stable/latest': 'v2.14.0'},
             ['v2.14.0+build-7', 'latest', 'v2', 'v2.14', 'stable', 'preview']),
            ('3.0.0', {'stable/latest': 'v2.99.99'}, ['v3.0.0', 'latest', 'v3', 'v3.0', 'stable', 'preview']),
        )
        for version, aliases, suffixes in cases:
            with self.subTest(version=version, aliases=aliases):
                result, fetch = self.generate(version, aliases)
                self.assert_tags(result, suffixes)
                major, minor = version.split('.')[:2]
                self.assertEqual([call.args[0] for call in fetch.call_args_list], [
                    f'{PREFIX}stable/{major}/Version', f'{PREFIX}stable/{major}.{minor}/Version',
                    f'{PREFIX}stable/latest/Version', f'{PREFIX}preview/latest/Version',
                ])
                self.assertEqual((result['nightly'], result['prerelease']), ('0', 'false'))

    def test_release_candidates(self):
        cases = (
            ('2.14.0-rc01', {}, ['v2.14.0-rc01', 'latest', 'preview']),
            ('2.14.0-rc02', {'preview/latest': 'v2.14.0-rc01'}, ['v2.14.0-rc02', 'latest', 'preview']),
            ('2.14.0-rc10', {'preview/latest': 'v2.14.0-rc09'}, ['v2.14.0-rc10', 'latest', 'preview']),
            ('2.14.0-rc02', {'preview/latest': 'v2.14.0'}, ['v2.14.0-rc02', 'latest']),
            ('2.14.0-rc02', {'preview/latest': 'v2.15.0-rc01'}, ['v2.14.0-rc02', 'latest']),
        )
        for version, aliases, suffixes in cases:
            with self.subTest(version=version, aliases=aliases):
                result, fetch = self.generate(version, aliases)
                self.assert_tags(result, suffixes)
                fetch.assert_called_once()
                self.assertEqual((result['nightly'], result['prerelease']), ('0', 'true'))

    def test_non_release_does_not_fetch(self):
        for version, suffixes in (('', ['test']), ('nightly', ['edge', 'latest'])):
            with self.subTest(version=version):
                result, fetch = self.generate(version)
                fetch.assert_not_called()
                self.assert_tags(result, suffixes)
                self.assertEqual(result['nightly'], '1' if version else '0')
                self.assertEqual(result['prerelease'], 'false')
                self.assertEqual(result['nightly_tag'], 'edge')

    def test_invalid_published_versions_fail(self):
        for value in ('', 'garbage', 'v2.12', 'v02.12.2', 'v2.12.2-01', 'v2.12.2+', 'v2.12.2 trailing'):
            with self.subTest(value=value), self.assertRaises(ValueError):
                self.generate('2.12.2', {'stable/2': value})

    def test_stable_followups_skip_prereleases(self):
        workflow = (SCRIPT.parent.parent / 'workflows/docker.yml').read_text()
        self.assertIn('prerelease: ${{ steps.tag.outputs.prerelease }}', workflow)
        for step_name in ('Trigger Helmchart PR', 'Trigger MSI build'):
            step = workflow.split(f'      - name: {step_name}\n', 1)[1]
            condition = step.split('        if: ', 1)[1].splitlines()[0]
            for version, should_run in (('2.13.0', True), ('2.13.0-rc01', False), ('nightly', False)):
                result, _ = self.generate(version)
                expression = condition.replace('github.event_name', repr('workflow_dispatch'))
                expression = expression.replace('github.event.inputs.version', repr(version))
                expression = expression.replace('github.repository', repr('netdata/netdata'))
                expression = expression.replace('needs.gen-tags.outputs.prerelease', repr(result['prerelease']))
                with self.subTest(step=step_name, version=version):
                    self.assertEqual(eval(expression.replace('&&', 'and'), {'__builtins__': {}}), should_run)

    def test_invalid_explicit_versions_fail(self):
        for version in ('garbage-rc', '2.12-rc.1', '2.12.2-01',
                        '2.12.2-rc..1', '02.12.2-rc.1'):
            with self.subTest(version=version), self.assertRaises(ValueError):
                self.generate(version, error=URLError('unavailable'))

    def test_network_failure_fails(self):
        with self.assertRaises(URLError):
            self.generate('2.12.2', error=URLError('unavailable'))


if __name__ == '__main__':
    unittest.main()

#!/usr/bin/env python3

import os
import json
import subprocess
from pathlib import Path
import runpy
import re
import tempfile
import unittest
from unittest.mock import patch


SCRIPT = Path(__file__).with_name('gen-docker-tags.py')
REPOS = ('netdata/netdata', 'quay.io/netdata/netdata', 'ghcr.io/netdata/netdata')


class DockerTagsTest(unittest.TestCase):
    def generate(self, version, published=None, error=None, *, registry=None, metadata_only=False):
        """Published maps image aliases (or full references for registry-specific state) to version labels."""
        published = {} if published is None else published

        def inspect(command, **kwargs):
            self.assertEqual(command[:8], ['skopeo', '--command-timeout', '60s', '--override-os', 'linux',
                                          '--override-arch', 'amd64', 'inspect'])
            self.assertEqual(command[8], '--no-tags')
            self.assertEqual(kwargs, {'capture_output': True, 'text': True, 'check': False, 'timeout': 90})
            reference = command[9].removeprefix('docker://')
            alias = reference.rsplit(':', 1)[1]
            if error is not None:
                return subprocess.CompletedProcess(command, 1, '', error)
            if reference in published:
                value = published[reference]
            elif alias in published:
                value = published[alias]
            else:
                return subprocess.CompletedProcess(command, 1, '',
                    f'reading manifest {alias} in {reference.rsplit(":", 1)[0]}: manifest unknown: not found')
            return subprocess.CompletedProcess(command, 0, json.dumps({
                'Labels': {'org.opencontainers.image.version': value}}), '')

        argv = [str(SCRIPT), 'workflow_dispatch', version]
        if registry:
            argv.extend(['--registry', registry])
        if metadata_only:
            argv.append('--metadata-only')
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / 'output'
            with patch.dict(os.environ, GITHUB_OUTPUT=str(output)), \
                 patch('sys.argv', argv), \
                 patch('subprocess.run', side_effect=inspect) as mock, \
                 patch('urllib.request.urlopen', side_effect=AssertionError('must not read artifact metadata')):
                runpy.run_path(str(SCRIPT), run_name='__main__')
            result = dict(line.split('=', 1) for line in output.read_text().splitlines())
        return result, mock

    def assert_tags(self, result, suffixes):
        self.assertEqual(result['tags'], ','.join(f'{repo}:{tag}' for repo in REPOS for tag in suffixes))
        for registry, repo in zip(('docker', 'quay', 'ghcr'), REPOS):
            self.assertEqual(result[f'{registry}_tags'], ','.join(f'{repo}:{tag}' for tag in suffixes))

    def test_stable_aliases_never_move_backwards(self):
        published = {'stable': 'v2.14.0', 'v2': 'v2.14.0', 'v2.13': 'v2.13.4',
                     'v2.14': 'v2.14.0', 'preview': 'v2.15.0-rc01'}
        cases = (
            # A patch for an older series only moves its own series alias.
            ('2.13.5', published, ['v2.13.5', 'latest', 'v2.13']),
            ('2.13.3', published, ['v2.13.3', 'latest']),
            ('2.14.1', published, ['v2.14.1', 'latest', 'v2', 'v2.14', 'stable']),
            ('2.15.0', published, ['v2.15.0', 'latest', 'v2', 'v2.15', 'stable', 'preview']),
            ('2.14.1', {}, ['v2.14.1', 'latest', 'v2', 'v2.14', 'stable', 'preview']),
            ('2.14.0', {'preview': 'v2.14.0-rc03'},
             ['v2.14.0', 'latest', 'v2', 'v2.14', 'stable', 'preview']),
            ('2.14.0+build-7', {'stable': 'v2.14.0'},
             ['v2.14.0+build-7', 'latest', 'v2', 'v2.14', 'stable', 'preview']),
            ('3.0.0', {'stable': 'v2.99.99'}, ['v3.0.0', 'latest', 'v3', 'v3.0', 'stable', 'preview']),
        )
        for version, aliases, suffixes in cases:
            with self.subTest(version=version, aliases=aliases):
                result, fetch = self.generate(version, aliases)
                self.assert_tags(result, suffixes)
                major, minor = version.split('.')[:2]
                self.assertEqual([call.args[0][-1] for call in fetch.call_args_list], [
                    f'docker://{repo}:{alias}' for repo in REPOS
                    for alias in (f'v{major}', f'v{major}.{minor}', 'stable', 'preview')
                ])
                self.assertEqual((result['nightly'], result['prerelease']), ('0', 'false'))

    def test_release_candidates(self):
        cases = (
            ('2.14.0-rc01', {}, ['v2.14.0-rc01', 'latest', 'preview']),
            ('2.14.0-rc02', {'preview': 'v2.14.0-rc01'}, ['v2.14.0-rc02', 'latest', 'preview']),
            ('2.14.0-rc10', {'preview': 'v2.14.0-rc09'}, ['v2.14.0-rc10', 'latest', 'preview']),
            ('2.14.0-rc02', {'preview': 'v2.14.0'}, ['v2.14.0-rc02', 'latest']),
            ('2.14.0-rc02', {'preview': 'v2.15.0-rc01'}, ['v2.14.0-rc02', 'latest']),
        )
        for version, aliases, suffixes in cases:
            with self.subTest(version=version, aliases=aliases):
                result, fetch = self.generate(version, aliases)
                self.assert_tags(result, suffixes)
                self.assertEqual(fetch.call_count, 3)
                self.assertEqual((result['nightly'], result['prerelease']), ('0', 'true'))

    def test_registry_state_is_independent(self):
        result, _ = self.generate('2.14.1', {
            'netdata/netdata:stable': '2.15.0',
            'quay.io/netdata/netdata:stable': '2.13.0',
            'ghcr.io/netdata/netdata:stable': '2.14.1',
        })
        self.assertNotIn('netdata/netdata:stable', result['tags'].split(','))
        for repo in REPOS[1:]:
            self.assertIn(f'{repo}:stable', result['tags'].split(','))

    def test_registry_selection(self):
        for registry, repo in zip(('docker', 'quay', 'ghcr'), REPOS):
            result, inspect = self.generate('2.14.1', registry=registry)
            self.assertTrue(all(tag.startswith(repo + ':') for tag in result['tags'].split(',')))
            self.assertEqual(inspect.call_count, 4)
            self.assertTrue(all(call.args[0][-1].startswith(f'docker://{repo}:')
                                for call in inspect.call_args_list))

    def test_build_metadata_does_not_check_aliases(self):
        result, inspect = self.generate('2.14.0-rc01', metadata_only=True)
        inspect.assert_not_called()
        self.assert_tags(result, ['v2.14.0-rc01', 'latest'])
        self.assertEqual(result['prerelease'], 'true')

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
                self.generate('2.12.2', {'v2': value})

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
                self.generate(version, error='unavailable')

    def test_inspection_failure_stops_publication(self):
        for error in ('unauthorized: authentication required', 'toomanyrequests: rate limit',
                      'dial tcp: timeout', 'blob unknown', 'manifest unknown without alias context',
                      'reading manifest sha256:abc in netdata/netdata: manifest unknown'):
            with self.subTest(error=error), self.assertRaises(RuntimeError):
                self.generate('2.12.2', error=error)

    def test_missing_alias_response(self):
        module = runpy.run_path(str(SCRIPT))
        for error in ('reading manifest stable in docker.io/netdata/netdata: manifest unknown: not found',
                      'fetching manifest stable in docker.io/netdata/netdata: name unknown: not found'):
            with self.subTest(error=error), patch('subprocess.run', return_value=
                    subprocess.CompletedProcess([], 1, '', error)):
                self.assertIsNone(module['published_version']('netdata/netdata', 'stable'))

    def test_missing_or_malformed_labels_stop_publication(self):
        for body in ('{}', '{"Labels":null}', '{"Labels":{}}', 'not json'):
            with self.subTest(body=body), patch('subprocess.run', return_value=
                    subprocess.CompletedProcess([], 0, body, '')):
                # Exercise the production reader directly, bypassing generate's CLI mock.
                module = runpy.run_path(str(SCRIPT))
                with self.assertRaises(ValueError):
                    module['published_version']('netdata/netdata', 'stable')

    def test_v_prefixed_version_does_not_double_prefix(self):
        result, _ = self.generate('v2.14.0', {'v2': '2.13.0', 'stable': '2.13.0', 'preview': '2.14.0-rc01'},
                                  registry='docker')
        self.assertEqual(result['docker_tags'],
                         'netdata/netdata:v2.14.0,netdata/netdata:latest,netdata/netdata:v2,'
                         'netdata/netdata:v2.14,netdata/netdata:stable,netdata/netdata:preview')
        rc, _ = self.generate('v2.14.0-rc01', {'preview': '2.13.0'}, registry='docker')
        self.assertEqual(rc['docker_tags'],
                         'netdata/netdata:v2.14.0-rc01,netdata/netdata:latest,netdata/netdata:preview')

    def test_publish_checks_are_serialized_and_used(self):
        workflow = (SCRIPT.parent.parent / 'workflows/docker.yml').read_text()
        for job, registry, prefix in (('publish-docker-hub', 'docker', "''"),
                                      ('publish-quay', 'quay', "'quay.io'"),
                                      ('publish-ghcr', 'ghcr', "'ghcr.io'")):
            # Select through the next job heading, rather than nested YAML indentation.
            block = re.split(r'\n  [a-z][a-z-]+:', workflow.split(f'  {job}:\n', 1)[1])[0]
            with self.subTest(registry=registry):
                self.assertIn(f'group: docker-publish-{registry}', block)
                self.assertIn('cancel-in-progress: false', block)
                self.assertIn('queue: max', block)
                self.assertIn(f'--registry {registry}', block)
                self.assertLess(block.index('id: login'), block.index('id: published-tags'))
                self.assertLess(block.index('id: published-tags'), block.index('id: manifest'))
                self.assertIn(f'/tmp/digests {prefix} "${{{{ steps.published-tags.outputs.tags }}}}"', block)
                self.assertNotIn('needs.gen-tags.outputs.tags', block)


if __name__ == '__main__':
    unittest.main()

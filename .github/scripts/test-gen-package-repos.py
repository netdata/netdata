#!/usr/bin/env python3

import contextlib
import io
import os
from pathlib import Path
import runpy
import subprocess
import tempfile
import unittest
from unittest.mock import MagicMock, patch
from urllib.error import HTTPError, URLError

SCRIPTS = Path(__file__).resolve().parent
WORKFLOW = (SCRIPTS.parent / 'workflows/packaging.yml').read_text()


def step_script(name):
    step = WORKFLOW.split(f'      - name: {name}\n', 1)[1].split('\n      - name:', 1)[0]
    lines = step.split('        run: |\n', 1)[1].splitlines()
    return '\n'.join(line[10:] for line in lines)


class PackageReposTest(unittest.TestCase):
    def generate(self, stable, preview, error=None):
        response = MagicMock()
        response.__enter__.return_value.read.return_value = preview.encode()
        output = io.StringIO()
        with patch('sys.argv', ['gen-package-repos.py', stable, 'netdata/netdata']), \
             patch('urllib.request.urlopen', return_value=response, side_effect=error), \
             contextlib.redirect_stdout(output):
            runpy.run_path(str(SCRIPTS / 'gen-package-repos.py'), run_name='__main__')
        return output.getvalue().strip()

    def test_semver_destinations(self):
        for stable, preview, eligible in (
            ('2.12.1', 'v2.12.2', False),
            ('2.12.2', 'v2.12.2\n', True),
            ('2.12.3', 'v2.12.2', True),
            ('2.9.9', 'v2.12.2', False),
            ('2.12.2', 'v2.9.9', True),
            ('3.0.0', 'v2.99.99', True),
            ('2.12.2', 'v2.12.2-rc.1+build.7', True),
            ('2.12.2', 'v2.12.2+build.7', True),
        ):
            with self.subTest(stable=stable, preview=preview):
                expected = 'netdata/netdata'
                if eligible:
                    expected += ' netdata/netdata-preview'
                self.assertEqual(self.generate(stable, preview), expected)

    def test_missing_preview_is_eligible(self):
        error = HTTPError('https://example.invalid/Version', 404, 'Not Found', None, None)
        self.assertEqual(self.generate('2.12.2', '', error),
                         'netdata/netdata netdata/netdata-preview')

    def test_lookup_failure(self):
        with self.assertRaises(URLError):
            self.generate('2.12.2', 'v2.12.2', error=URLError('offline'))
        with self.assertRaises(ValueError):
            self.generate('2.12.2', 'invalid')

    def test_workflow_channels(self):
        for channel, version, expected in (
            ('release', '2.12.1', 'netdata/netdata'),
            ('release', '2.12.2', 'netdata/netdata netdata/netdata-preview'),
            ('release', '2.12.3', 'netdata/netdata netdata/netdata-preview'),
            ('nightly', '2.12.2', 'netdata/netdata-edge'),
            ('devel', '2.12.2', 'netdata/netdata-devel'),
        ):
            script = step_script('Check Version')
            script = script.replace('${{ github.event_name }}', 'workflow_dispatch')
            script = script.replace('${{ github.event.inputs.type }}', channel)
            script = script.replace('${{ github.event.inputs.version }}', version)
            with self.subTest(channel=channel, version=version), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                (root / 'packaging').mkdir()
                (root / 'packaging/version').write_text('v2.12.2-1-nightly')
                (root / '.github').mkdir()
                (root / '.github/scripts').symlink_to(SCRIPTS, target_is_directory=True)
                preview = root / 'Version'
                preview.write_text('v2.12.2\n')
                # Keep the real workflow command and eligibility logic; redirect only
                # the child Python process's preview lookup to fixed local metadata.
                (root / 'sitecustomize.py').write_text(
                    'from modules import release_versions\n'
                    f'release_versions.PREVIEW_VERSION_URL = {preview.as_uri()!r}\n'
                )
                output = root / 'output'
                subprocess.run(['bash', '-eu', '-c', script], cwd=root, check=True,
                               env={**os.environ, 'GITHUB_OUTPUT': str(output),
                                    'PYTHONPATH': os.pathsep.join((str(root), str(SCRIPTS))),
                                    'REPO_PREFIX': 'netdata/netdata', 'RELEASE_VERSION': version,
                                    'GITHUB_SHA': 'abcdef'})
                result = dict(line.split('=', 1) for line in output.read_text().splitlines())
                self.assertEqual(result['repos'], expected)

    def test_both_hosts_and_formats(self):
        for host in ('packages.netdata.cloud', 'packages2.netdata.cloud'):
            for fmt in ('deb', 'rpm'):
                for repos in ('netdata/netdata', 'netdata/netdata netdata/netdata-preview'):
                    with self.subTest(host=host, fmt=fmt, repos=repos):
                        script = step_script(f'Upload to {host}')
                        for key, value in (('repo_distro', 'test/distro'), ('arch', 'amd64'), ('format', fmt)):
                            script = script.replace('${{ matrix.' + key + ' }}', value)
                        script = script.replace('.github/scripts/package-upload.sh', 'record_upload')
                        script = 'record_upload() { printf "%s\\n" "$*"; }\n' + script
                        result = subprocess.run(['bash', '-eu', '-c', script], check=True,
                                                capture_output=True, text=True,
                                                env={**os.environ, 'PACKAGE_REPOS': repos})
                        self.assertEqual(result.stdout.splitlines(), [
                            f'{host} test/distro amd64 {fmt} {repo}' for repo in repos.split()
                        ])


if __name__ == '__main__':
    unittest.main()

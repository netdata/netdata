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
from urllib.error import URLError

SCRIPTS = Path(__file__).resolve().parent
WORKFLOW = (SCRIPTS.parent / 'workflows/build.yml').read_text()
JOB = WORKFLOW.split('  publish-stable-artifacts:', 1)[1].split('\n  # Remaining jobs', 1)[0]


def step_script(name):
    step = JOB.split(f'      - name: {name}\n', 1)[1].split('\n      - name:', 1)[0]
    return '\n'.join(line[10:] for line in step.split('        run: |\n', 1)[1].splitlines())


class ArtifactPrefixesTest(unittest.TestCase):
    def eligibility(self, stable, preview, error=None):
        response = MagicMock()
        response.__enter__.return_value.read.return_value = preview.encode()
        output = io.StringIO()
        with patch('sys.argv', ['release_versions.py', stable]), \
             patch('urllib.request.urlopen', return_value=response, side_effect=error), \
             contextlib.redirect_stdout(output):
            runpy.run_path(str(SCRIPTS / 'modules/release_versions.py'), run_name='__main__')
        return output.getvalue().strip()

    def select(self, eligibility, exit_code=0):
        script = step_script('Select Artifact Prefixes').replace(
            'python3 .github/scripts/modules/release_versions.py', 'lookup_preview',
        )
        script = (f'lookup_preview() {{ echo {eligibility}; return {exit_code}; }}\n' + script)
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / 'output'
            result = subprocess.run(['bash', '-eu', '-c', script], capture_output=True,
                                    env={**os.environ, 'RELEASE_VERSION': '2.12.2',
                                         'GITHUB_OUTPUT': str(output)})
            return result.returncode, output.read_text() if output.exists() else ''

    def test_comparison_and_selection(self):
        for stable, preview, expected in (
            ('2.12.1', 'v2.12.2', 'stable'),
            ('2.12.2', 'v2.12.2\n', 'stable preview'),
            ('2.12.3', 'v2.12.2', 'stable preview'),
            ('2.9.9', 'v2.12.2', 'stable'),
            ('3.0.0', 'v2.99.99', 'stable preview'),
            ('2.12.2', 'v2.12.2-rc.1', 'stable preview'),
            ('2.12.2', 'v2.12.2+build.9', 'stable preview'),
        ):
            with self.subTest(stable=stable, preview=preview):
                self.assertEqual(self.select(self.eligibility(stable, preview)),
                                 (0, f'prefixes={expected}\n'))

    def test_lookup_errors_stop_selection(self):
        with self.assertRaises(URLError):
            self.eligibility('2.12.2', '', URLError('offline'))
        with self.assertRaises(ValueError):
            self.eligibility('2.12.2', 'invalid')
        code, output = self.select('false', exit_code=1)
        self.assertNotEqual(code, 0)
        self.assertEqual(output, '')

    def test_upload_directories_and_failure(self):
        script = step_script('Upload Artifacts')
        stub = 'rclone() { printf "%s|%s\\n" "${7}" "${8}"; }\n'
        with tempfile.TemporaryDirectory() as directory:
            dirs = ('latest', 'v2.12.2', '2', '2.12')
            for name in dirs:
                (Path(directory) / name).mkdir()
            for prefixes in ('stable', 'stable preview'):
                with self.subTest(prefixes=prefixes):
                    result = subprocess.run(['bash', '-eu', '-c', stub + script],
                                            cwd=directory, capture_output=True, text=True, check=True,
                                            env={**os.environ, 'ARTIFACT_PREFIXES': prefixes})
                    self.assertCountEqual(result.stdout.splitlines(), [
                        f'{name}/|s3:netdata-agent-artifacts/{prefix}/{name}/'
                        for prefix in prefixes.split() for name in sorted(dirs)
                    ])
            failure = subprocess.run(['bash', '-eu', '-c', 'rclone() { return 1; }\n' + script],
                                     cwd=directory, env={**os.environ, 'ARTIFACT_PREFIXES': 'stable preview'})
            self.assertNotEqual(failure.returncode, 0)


if __name__ == '__main__':
    unittest.main()

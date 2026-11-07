#!/usr/bin/env python3

import contextlib
import io
import hashlib
import json
import os
from pathlib import Path
import runpy
import subprocess
import tempfile
import unittest
from unittest.mock import MagicMock, patch
from urllib.error import HTTPError, URLError

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

    def test_missing_preview_is_eligible(self):
        error = HTTPError('https://example.invalid/Version', 404, 'Not Found', None, None)
        self.assertEqual(self.select(self.eligibility('2.12.2', '', error)),
                         (0, 'prefixes=stable preview\n'))
        with self.assertRaises(ValueError):
            self.eligibility('invalid', '', error)

    def test_other_http_errors_propagate(self):
        for status in (403, 500):
            error = HTTPError('https://example.invalid/Version', status, 'Failure', None, None)
            with self.subTest(status=status), self.assertRaises(HTTPError) as caught:
                self.eligibility('2.12.2', '', error)
            self.assertIs(caught.exception, error)
            error.close()

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
            root = Path(directory)
            stable = root / 'artifacts'
            preview = root / 'preview-artifacts'
            trees = {'stable': (stable, ('v2.12.2',)),
                     'preview': (preview, ('latest', 'v2.12.2', '2', '2.12'))}
            for tree, dirs in trees.values():
                for name in dirs:
                    (tree / name).mkdir(parents=True)
            for prefixes in ('stable', 'stable preview'):
                with self.subTest(prefixes=prefixes):
                    result = subprocess.run(['bash', '-eu', '-c', stub + script],
                                            cwd=stable, capture_output=True, text=True, check=True,
                                            env={**os.environ, 'ARTIFACT_PREFIXES': prefixes})
                    self.assertCountEqual(result.stdout.splitlines(), [
                        f'{"./" if prefix == "stable" else "../preview-artifacts/"}{name}/|'
                        f's3:netdata-agent-artifacts/{prefix}/{name}/'
                        for prefix in prefixes.split() for name in trees[prefix][1]
                    ])
            failure = subprocess.run(['bash', '-eu', '-c', 'rclone() { return 1; }\n' + script],
                                     cwd=stable, env={**os.environ, 'ARTIFACT_PREFIXES': 'stable preview'})
            self.assertNotEqual(failure.returncode, 0)


class PrepareArtifactAliasesTest(unittest.TestCase):
    def prepare(self, stable, preview, event='workflow_dispatch', build_type='release'):
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        root = Path(directory.name)
        (root / 'packaging').mkdir()
        (root / 'packaging/version').write_text('v2.12.2')
        source = root / 'source'
        source.mkdir()
        files = ['netdata-v2.12.2.tar.gz', 'netdata-v2.12.2.tar.zst', 'netdata-x64.msi']
        files += [f'netdata-{arch}-latest.gz.run'
                  for arch in ('x86_64', 'aarch64', 'armv6l', 'armv7l')]
        for name in files:
            (source / name).write_text('synthetic artifact')
        binaries = root / 'bin'
        binaries.mkdir()
        wget = binaries / 'wget'
        wget.write_text('#!/usr/bin/env python3\n'
                        'import json, pathlib, sys\n'
                        f'versions = json.loads({json.dumps({"stable": stable, "preview": preview})!r})\n'
                        'url = sys.argv[-1]\n'
                        'pathlib.Path("lookups").open("a").write(url + "\\n")\n'
                        'channel, alias, _ = url.rsplit("/", 3)[1:]\n'
                        'value = versions.get(channel, {}).get(alias)\n'
                        'if value is None:\n'
                        '    pathlib.Path(sys.argv[sys.argv.index("-o") + 1]).write_text("HTTP/1.1 404 Not Found\\n")\n'
                        '    sys.exit(8)\n'
                        'pathlib.Path("Version").write_text(value)\n')
        wget.chmod(0o755)
        gpg = binaries / 'gpg'
        gpg.write_text('#!/bin/sh\nprintf "synthetic signature" > Manifest.sig\n')
        gpg.chmod(0o755)
        result = subprocess.run(['bash', str(SCRIPTS / 'prepare-artifacts.sh'),
                                 str(source), 'test-key', event, build_type],
                                cwd=root, capture_output=True, text=True,
                                env={**os.environ, 'PATH': f'{binaries}:{os.environ["PATH"]}'})
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        return root

    def assert_tree(self, root, tree, aliases):
        directory = root / 'artifacts' / tree
        self.assertCountEqual([p.name for p in directory.iterdir()], ['v2.12.2', *aliases])
        for target in directory.iterdir():
            self.assertEqual((target / 'Version').read_text().strip(), 'v2.12.2')
            self.assertTrue((target / 'Manifest.sig').is_file())
            for line in (target / 'Manifest').read_text().splitlines():
                name, size, digest = line.split()
                data = (target / name).read_bytes()
                self.assertEqual(len(data), int(size))
                self.assertEqual(hashlib.sha256(data).hexdigest(), digest)

    def test_channels_select_aliases_independently(self):
        root = self.prepare(
            {'latest': 'v2.12.3', '2': 'v2.12.3', '2.12': 'v2.12.3'},
            {'latest': 'v2.12.1', '2': 'v2.12.1', '2.12': 'v2.12.1'},
        )
        self.assert_tree(root, 'r2', [])
        self.assert_tree(root, 'r2-preview', ['latest', '2', '2.12'])

    def test_aliases_use_their_own_metadata(self):
        root = self.prepare({}, {'latest': 'v3.0.0', '2': 'v2.12.2', '2.12': 'v2.12.2-rc1'})
        self.assert_tree(root, 'r2', ['latest', '2', '2.12'])
        self.assert_tree(root, 'r2-preview', ['2.12'])

    def test_nightly_and_pull_request_skip_preview(self):
        for event, build_type in (('workflow_dispatch', 'nightly'), ('pull_request', 'release')):
            with self.subTest(event=event, build_type=build_type):
                root = self.prepare({}, {}, event, build_type)
                self.assert_tree(root, 'r2', ['latest'])
                self.assertFalse((root / 'artifacts/r2-preview').exists())
                self.assertFalse((root / 'lookups').exists())


if __name__ == '__main__':
    unittest.main()

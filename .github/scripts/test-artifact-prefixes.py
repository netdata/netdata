#!/usr/bin/env python3

import contextlib
import io
import hashlib
import json
import os
from pathlib import Path
import runpy
import re
import shutil
import subprocess
import tempfile
import unittest
from unittest.mock import MagicMock, patch
from urllib.error import HTTPError, URLError

SCRIPTS = Path(__file__).resolve().parent
WORKFLOW = (SCRIPTS.parent / 'workflows/build.yml').read_text()
JOB = WORKFLOW.split('  publish-stable-artifacts:', 1)[1].split('\n  # Remaining jobs', 1)[0]


def step_script(name, job=JOB):
    step = job.split(f'      - name: {name}\n', 1)[1].split('\n      - name:', 1)[0]
    run = step.split('        run: ', 1)[1]
    if run.startswith('|\n'):
        return '\n'.join(line[10:] for line in run[2:].splitlines())
    return run.splitlines()[0]


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

    def select(self, eligibility, exit_code=0, build_type="release"):
        script = step_script('Select Artifact Prefixes').replace(
            'python3 .github/scripts/modules/release_versions.py', 'lookup_preview',
        )
        script = (f'lookup_preview() {{ echo {eligibility}; return {exit_code}; }}\n' + script)
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / 'output'
            result = subprocess.run(['bash', '-eu', '-c', script], capture_output=True,
                                    env={**os.environ, 'RELEASE_VERSION': '2.12.2', 'RELEASE_TYPE': build_type,
                                         'GITHUB_OUTPUT': str(output)})
            return result.returncode, output.read_text() if output.exists() else ''

    def test_github_tag_normalization(self):
        step = WORKFLOW.split('      - name: Normalize Tag\n', 1)[1].split('  upload-release:', 1)[0]
        script = '\n'.join(line[10:] for line in step.split('        run: |\n', 1)[1].splitlines())
        for version in ('2.13.0', '2.13.0-rc01', 'v2.13.0', 'v2.13.0-rc01'):
            with self.subTest(version=version), tempfile.TemporaryDirectory() as directory:
                output = Path(directory) / 'output'
                subprocess.run(['bash', '-eu', '-c', script], check=True,
                               env={**os.environ, 'RELEASE_VERSION': version, 'GITHUB_OUTPUT': str(output)})
                self.assertEqual(output.read_text(), f'tag=v{version.removeprefix("v")}\n')

    def test_installers_use_requested_channel(self):
        steps = re.findall(r'      - name: (?:Mark Stable|Set Release Channel)\n(.*?)(?=      - name:)',
                           WORKFLOW, re.S)
        self.assertEqual(len(steps), 2)
        for kind, expected in (('release', 'stable'), ('release-candidate', 'preview')):
            for step in steps:
                script = '\n'.join(line[10:] for line in step.split('        run: |\n', 1)[1].splitlines())
                with self.subTest(kind=kind, script=script), tempfile.TemporaryDirectory() as directory:
                    root = Path(directory)
                    (root / 'packaging/makeself').mkdir(parents=True)
                    paths = [root / 'netdata-installer.sh', root / 'packaging/makeself/install-or-update.sh']
                    for path in paths:
                        path.write_text('RELEASE_CHANNEL="nightly"\n')
                    subprocess.run(['bash', '-eu', '-c', script], cwd=root, check=True,
                                   env={**os.environ, 'RELEASE_TYPE': kind})
                    self.assertEqual(paths[0].read_text(), f'RELEASE_CHANNEL="{expected}"\n')
                    if 'install-or-update.sh' in script:
                        self.assertEqual(paths[1].read_text(), paths[0].read_text())

    def test_rc_selects_preview_without_stable_lookup(self):
        self.assertEqual(self.select('false', exit_code=1, build_type='release-candidate'),
                         (0, 'prefixes=preview\n'))

    def upload(self, root, prefixes, release_type, published=None, empty_listing=False, fail_sync=False):
        """Run the upload step from ROOT/artifacts; PUBLISHED maps '<prefix>/<alias>' to the bucket's Version."""
        bucket = root / 'bucket'
        for key, value in (published or {}).items():
            (bucket / key).mkdir(parents=True)
            (bucket / key / 'Version').write_text(value + '\n')
        missing = 'return 0' if empty_listing else 'return 3'
        stub = (
            'rclone() {\n'
            '  if [ "$1" = cat ]; then\n'
            f'    f="{bucket}/${{2#s3:netdata-agent-artifacts/}}"\n'
            f'    if [ -f "$f" ]; then cat "$f"; else {missing}; fi\n'
            '    return\n'
            '  fi\n'
            f'  {"return 1" if fail_sync else "printf \"%s|%s\\n\" \"${7}\" \"${8}\""}\n'
            '}\n'
        )
        script = step_script('Upload Artifacts').replace(
            '../.github/scripts/modules/release_versions.py', str(SCRIPTS / 'modules/release_versions.py'))
        return subprocess.run(['bash', '-eu', '-c', stub + script], cwd=root / 'artifacts',
                              capture_output=True, text=True,
                              env={**os.environ, 'ARTIFACT_PREFIXES': prefixes, 'RELEASE_TYPE': release_type})

    @staticmethod
    def tree(path, version, aliases):
        for name in (version, *aliases):
            (path / name).mkdir(parents=True)
            (path / name / 'Version').write_text(version + '\n')

    def test_publication_lock_is_shared_and_queued(self):
        lock = JOB.split('    concurrency:', 1)[1].split('    steps:', 1)[0]
        self.assertEqual([line.split('#')[0].strip() for line in lock.strip().splitlines()][1:],
                         ['group: r2-release-publish', 'cancel-in-progress: false', 'queue: max'])

    def test_rc_uploads_only_preview(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            self.tree(root / 'artifacts', 'v2.13.0-rc01', ('latest',))
            result = self.upload(root, 'preview', 'release-candidate')
            self.assertEqual(result.returncode, 0, result.stderr)
            # The versioned directory publishes before its alias.
            self.assertEqual(result.stdout.splitlines(), [
                f'./{name}/|s3:netdata-agent-artifacts/preview/{name}/' for name in ('v2.13.0-rc01', 'latest')
            ])

    def test_alias_recheck_under_lock(self):
        # Preparation saw rc01 under preview/latest; a faster RC03 publisher moved it before this RC02 upload.
        cases = {
            'newer published alias is kept': ({'preview/latest': 'v2.13.0-rc03'}, False, ['v2.13.0-rc02']),
            'older published alias moves': ({'preview/latest': 'v2.13.0-rc01'}, False,
                                            ['v2.13.0-rc02', 'latest']),
            'same release republishes': ({'preview/latest': 'v2.13.0-rc02'}, False, ['v2.13.0-rc02', 'latest']),
            'missing alias (not found)': ({}, False, ['v2.13.0-rc02', 'latest']),
            'missing alias (empty listing)': ({}, True, ['v2.13.0-rc02', 'latest']),
        }
        for case, (published, empty_listing, expected) in cases.items():
            with self.subTest(case=case), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                self.tree(root / 'artifacts', 'v2.13.0-rc02', ('latest',))
                result = self.upload(root, 'preview', 'release-candidate', published, empty_listing)
                self.assertEqual(result.returncode, 0, result.stderr)
                uploads = [line for line in result.stdout.splitlines() if not line.startswith('::notice::')]
                self.assertEqual(uploads, [
                    f'./{name}/|s3:netdata-agent-artifacts/preview/{name}/' for name in expected
                ])

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
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            self.tree(root / 'artifacts', 'v2.12.2', ())
            self.tree(root / 'preview-artifacts', 'v2.12.2', ('latest', '2', '2.12'))
            trees = {'stable': ('v2.12.2',), 'preview': ('v2.12.2', 'latest', '2', '2.12')}
            for prefixes in ('stable', 'stable preview'):
                with self.subTest(prefixes=prefixes):
                    result = self.upload(root, prefixes, 'release')
                    self.assertEqual(result.returncode, 0, result.stderr)
                    self.assertCountEqual(result.stdout.splitlines(), [
                        f'{"./" if prefix == "stable" else "../preview-artifacts/"}{name}/|'
                        f's3:netdata-agent-artifacts/{prefix}/{name}/'
                        for prefix in prefixes.split() for name in trees[prefix]
                    ])
            failure = self.upload(root, 'stable preview', 'release', fail_sync=True)
            self.assertNotEqual(failure.returncode, 0)
            # A bucket read error stops publication instead of treating the alias as unpublished.
            (root / 'bucket').mkdir(exist_ok=True)
            broken = self.upload(root, 'preview', 'release', {'preview/latest': 'garbage'})
            self.assertNotEqual(broken.returncode, 0)


class PrepareArtifactAliasesTest(unittest.TestCase):
    def test_installer_checks_use_versioned_artifacts(self):
        cases = {
            'older RC': ('release-candidate', 'v2.13.0-rc01', {}, {'latest': 'v2.13.0-rc02'}),
            'repeated RC': ('release-candidate', 'v2.13.0-rc01', {}, {'latest': 'v2.13.0-rc01'}),
            'older stable': ('release', 'v2.12.2', {'latest': 'v2.13.0'}, {'latest': 'v2.13.0'}),
            'nightly': ('nightly', 'v2.14-37-nightly', {}, {}),
        }
        for name, (kind, version, stable, preview) in cases.items():
            root = self.prepare(stable, preview, build_type=kind, version=version)
            if name in ('older RC', 'older stable'):
                self.assertFalse((root / 'artifacts/r2/latest').exists())
            else:
                self.assertTrue((root / 'artifacts/r2/latest').exists())
            for job_name in ('artifact-verification-dist', 'artifact-verification-static'):
                job = re.split(r'\n  [a-z][a-z-]+:', WORKFLOW.split(f'  {job_name}:', 1)[1])[0]
                with self.subTest(case=name, job=job_name), tempfile.TemporaryDirectory() as directory:
                    checkout = Path(directory)
                    (checkout / 'packaging').mkdir()
                    (checkout / 'packaging/version').write_text(version + '\n')
                    shutil.copytree(root / 'artifacts/r2', checkout / 'artifacts')
                    subprocess.run(['bash', '-eu', '-c', step_script('Prepare artifacts directory', job)],
                                   cwd=checkout, check=True, capture_output=True)
                    # Intercept installation, keeping the real workflow's URL selection and staging.
                    env = {**os.environ, 'NETDATA_ARTIFACT_URL_PREFIX': 'http://localhost:8080/latest'}
                    result = subprocess.run(['bash', '-eu', '-c',
                                             'sh() { printf "%s\\n" "$NETDATA_ARTIFACT_URL_PREFIX"; }\n'
                                             + step_script('Verify that artifacts work with installer', job)],
                                            cwd=checkout, env=env, check=True, capture_output=True, text=True)
                    self.assertEqual(result.stdout.strip(), f'http://localhost:8080/{version}')
                    selected = checkout / version
                    self.assertEqual((selected / 'Version').read_text().strip(), version)
                    self.assertTrue((selected / 'Manifest.sig').is_file())
                    for record in (selected / 'Manifest').read_text().splitlines():
                        filename, size, digest = record.split()
                        data = (selected / filename).read_bytes()
                        self.assertEqual((len(data), hashlib.sha256(data).hexdigest()), (int(size), digest))

    def prepare(self, stable, preview, event='workflow_dispatch', build_type='release', version='v2.12.2'):
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        root = Path(directory.name)
        (root / 'packaging').mkdir()
        (root / 'packaging/version').write_text(version)
        source = root / 'source'
        source.mkdir()
        files = [f'netdata-{version}.tar.gz', f'netdata-{version}.tar.zst', 'netdata-x64.msi']
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

    def assert_tree(self, root, tree, aliases, version='v2.12.2'):
        directory = root / 'artifacts' / tree
        self.assertCountEqual([p.name for p in directory.iterdir()], [version, *aliases])
        for target in directory.iterdir():
            self.assertEqual((target / 'Version').read_text().strip(), version)
            self.assertTrue((target / 'Manifest.sig').is_file())
            for line in (target / 'Manifest').read_text().splitlines():
                name, size, digest = line.split()
                data = (target / name).read_bytes()
                self.assertEqual(len(data), int(size))
                self.assertEqual(hashlib.sha256(data).hexdigest(), digest)

    def test_rc_latest_handles_large_numbers(self):
        version = 'v2.13.0-rc9223372036854775808'
        root = self.prepare({}, {'latest': 'v2.13.0-rc9223372036854775807'},
                            build_type='release-candidate', version=version)
        self.assert_tree(root, 'r2', ['latest'], version=version)

    def test_rc_artifacts_use_preview_metadata_only(self):
        root = self.prepare({}, {}, build_type='release-candidate', version='v2.13.0-rc01')
        self.assert_tree(root, 'r2', ['latest'], version='v2.13.0-rc01')
        self.assertFalse((root / 'artifacts/r2-preview').exists())
        lookups = (root / 'lookups').read_text().splitlines()
        self.assertEqual(lookups, ['https://artifacts.netdata.cloud/preview/latest/Version'])

    def test_channels_select_aliases_independently(self):
        root = self.prepare(
            {'latest': 'v2.12.3', '2': 'v2.12.3', '2.12': 'v2.12.3'},
            {'latest': 'v2.12.1', '2': 'v2.12.1', '2.12': 'v2.12.1'},
        )
        self.assert_tree(root, 'r2', [])
        self.assert_tree(root, 'r2-preview', ['latest', '2', '2.12'])

    def test_aliases_use_their_own_metadata(self):
        root = self.prepare({}, {'latest': 'v3.0.0', '2': 'v2.12.3', '2.12': 'v2.12.2-rc01'})
        self.assert_tree(root, 'r2', ['latest', '2', '2.12'])
        self.assert_tree(root, 'r2-preview', ['2.12'])

    def test_republishing_same_version_updates_aliases(self):
        published = {'latest': 'v2.12.2', '2': 'v2.12.2', '2.12': 'v2.12.2'}
        root = self.prepare(published, published)
        self.assert_tree(root, 'r2', ['latest', '2', '2.12'])
        self.assert_tree(root, 'r2-preview', ['latest', '2', '2.12'])

    def test_nightly_and_pull_request_skip_preview(self):
        for event, build_type in (('workflow_dispatch', 'nightly'), ('pull_request', 'release')):
            with self.subTest(event=event, build_type=build_type):
                root = self.prepare({}, {}, event, build_type)
                self.assert_tree(root, 'r2', ['latest'])
                self.assertFalse((root / 'artifacts/r2-preview').exists())
                self.assertFalse((root / 'lookups').exists())


if __name__ == '__main__':
    unittest.main()

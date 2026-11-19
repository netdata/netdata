#!/usr/bin/env python3
"""Exercise release preparation against real, isolated Git refs."""

import os
from pathlib import Path
import subprocess
import tempfile
import unittest

SCRIPT = Path(__file__).with_name('prepare-release-base.sh').resolve()


class ReleasePreparationTest(unittest.TestCase):
    def setUp(self):
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        self.root = Path(directory.name)
        self.git('init', '-b', 'master')
        self.git('config', 'commit.gpgsign', 'false')
        self.git('config', 'tag.gpgsign', 'false')
        self.git('config', 'user.name', 'Release test')
        self.git('config', 'user.email', 'release@example.invalid')
        (self.root / 'packaging').mkdir()
        self.add_release_tooling()
        self.write_version('v2.12.3-1-nightly')
        self.git('tag', 'v2.12.3')
        self.output = self.root / 'output'

    def add_release_tooling(self):
        """Minimal stand-ins for the tooling a release branch needs before its tag is pushed."""
        workflows = self.root / '.github/workflows'
        workflows.mkdir(parents=True)
        (workflows / 'dem-runtime-release.yml').write_text('run: python3 packaging/release_info.py\n')
        (self.root / 'packaging/release_info.py').write_text('')
        for name in ('build', 'docker', 'packaging'):
            (workflows / f'{name}.yml').write_text(
                'on:\n  workflow_dispatch:\n    inputs:\n      source_sha:\n      companion_run_id:\n')
        self.git('add', '.github', 'packaging/release_info.py')

    def git(self, *args):
        return subprocess.run(['git', *args], cwd=self.root, check=True,
                              capture_output=True, text=True).stdout.strip()

    def write_version(self, version):
        (self.root / 'packaging/version').write_text(version + '\n')
        self.git('add', 'packaging/version')
        self.git('commit', '--allow-empty', '-m', version)

    def series(self, version='v2.13.0-rc01', remote=False):
        branch = 'release-' + '.'.join(version.split('.')[:2])
        self.git('checkout', '-b', branch)
        self.write_version(version)
        self.git('tag', '-a', version, '-m', version)
        head = self.git('rev-parse', 'HEAD')
        self.git('checkout', 'master')
        if remote:
            self.git('update-ref', f'refs/remotes/origin/{branch}', head)
            self.git('branch', '-D', branch)
            self.git('config', 'remote.origin.url', str(self.root))
            self.git('config', 'remote.origin.fetch', '+refs/heads/*:refs/remotes/origin/*')

    def prepare(self, version, kind='release-candidate', event='workflow_dispatch', repo='netdata/netdata'):
        self.output.write_text('')
        result = subprocess.run(['sh', str(SCRIPT), repo, event, kind, version, ''],
                                cwd=self.root, capture_output=True, text=True,
                                env={**os.environ, 'GITHUB_OUTPUT': str(self.output)})
        outputs = dict(line.split('=', 1) for line in self.output.read_text().splitlines())
        return result, outputs

    def assert_rejected(self, version, kind='release-candidate', error=None):
        before = (self.root / 'packaging/version').read_text()
        result, outputs = self.prepare(version, kind)
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(outputs, {})
        self.assertEqual((self.root / 'packaging/version').read_text(), before)
        if error:
            self.assertIn(error, result.stdout)

    def test_first_rc(self):
        for version in ('v2.13.0-rc01', 'v3.0.0-rc01'):
            with self.subTest(version=version):
                result, outputs = self.prepare(version)
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                branch = 'release-' + '.'.join(version.split('.')[:2])
                major, minor = branch.removeprefix('release-v').split('.')
                self.assertEqual(outputs, {
                    'run': 'true', 'message': f'release-candidate release {version}.',
                    'ref': version, 'type': 'release-candidate', 'channel': 'release-candidate', 'branch': branch,
                    'new-branch': branch, 'marker': f'v{major}.{int(minor) + 1}',
                    'version': version[1:],
                })
                self.assertEqual((self.root / 'packaging/version').read_text(), version + '\n')
                # The workflow creates the branch and marker after committing the version/changelog.
                self.assertEqual(self.git('branch', '--list', outputs['new-branch']), '')
                (self.root / 'packaging/version').write_text('v2.12.3-1-nightly\n')

    def test_rc_input_is_normalized(self):
        for version in ('v2.13.0~rc1', 'v2.13.0-rc1', 'v2.13.0~rc01'):
            with self.subTest(version=version):
                result, outputs = self.prepare(version)
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                self.assertEqual((outputs['ref'], outputs['version'], outputs['new-branch']),
                                 ('v2.13.0-rc01', '2.13.0-rc01', 'release-v2.13'))
                self.assertEqual((self.root / 'packaging/version').read_text(), 'v2.13.0-rc01\n')
                (self.root / 'packaging/version').write_text('v2.12.3-1-nightly\n')

    def test_invalid_rc_versions(self):
        for version in ('v2.13.0', '2.13.0-rc01', 'v2.13.1-rc01', 'v2.13.0-rc0', 'v2.13.0-rc00',
                        'v2.13.0-rc001', 'v2.13.0-rc100', 'v2.13.0-rc-1', 'v2.13.0-rc.1',
                        'v02.13.0-rc01', 'v2.013.0-rc01', 'v2.13.00-rc01',
                        'v2.13.0-rc01+build.7', 'v2.13.0-beta1', 'v2.13.0-rc01.extra',
                        'v2.13.0-rc01\nrun=true', 'v2.13.0~rc0', 'v2.13.0~rc1~rc2',
                        'v2.13.0~rc1+build.7', 'v2.13.1~rc1'):
            with self.subTest(version=version):
                self.assert_rejected(version)

    def test_first_number_and_new_series(self):
        for version in ('v2.13.0-rc02', 'v2.12.0-rc01', 'v1.99.0-rc01', 'v3.1.0-rc01'):
            with self.subTest(version=version):
                self.assert_rejected(version)

    def test_first_rc_against_marker(self):
        self.git('tag', '-a', 'v2.14', '-m', 'marker')
        self.git('commit', '--allow-empty', '-m', 'change')
        for version in ('v2.13.0-rc01', 'v2.12.0-rc01'):
            with self.subTest(version=version):
                self.assert_rejected(version, error='not newer')
        for version, marker in (('v2.14.0-rc01', 'v2.15'), ('v3.0.0-rc01', 'v3.1')):
            with self.subTest(version=version):
                result, outputs = self.prepare(version)
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                self.assertEqual(outputs['marker'], marker)
                (self.root / 'packaging/version').write_text('v2.12.3-1-nightly\n')

    def test_rc_after_stable(self):
        for stable in ('v2.13.0', 'v2.13.0+build.7'):
            with self.subTest(stable=stable):
                self.git('tag', stable)
                self.assert_rejected('v2.13.0-rc01', error='regular release')

    def test_later_rc_numbers_and_remote_branch(self):
        self.git('tag', '-a', 'v2.13', '-m', 'development marker')
        self.series('v2.13.0-rc09', remote=True)
        self.git('tag', 'v2.13.0-rc01')
        for version in ('v2.13.0-rc09', 'v2.13.0-rc02', 'v2.13.0-rc08', 'v2.13.0-rc8'):
            with self.subTest(version=version):
                self.assert_rejected(version)
        for version in ('v2.13.0-rc10', 'v2.13.0-rc15', 'v2.13.0-rc99'):
            with self.subTest(version=version):
                result, outputs = self.prepare(version)
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                self.assertEqual(outputs['branch'], 'release-v2.13')
                self.assertEqual(outputs['type'], 'release-candidate')
                self.assertNotIn('new-branch', outputs)
                self.assertNotIn('marker', outputs)
                self.write_version(version)
                self.git('tag', version)
        self.assert_rejected('v2.13.0-rc16', error='newer than')

    def test_missing_branch(self):
        self.git('tag', 'v2.13.0-rc01')
        for kind, version in (('release-candidate', 'v2.13.0-rc02'),
                              ('minor', 'v2.13.0'), ('major', 'v3.0.0'),
                              ('patch', 'v2.13.1')):
            with self.subTest(kind=kind):
                self.assert_rejected(version, kind, 'Could not find a branch')

    def test_first_rc_rejects_existing_branch_or_marker(self):
        self.git('branch', 'release-v2.13')
        self.assert_rejected('v2.13.0-rc01', error='already exists')
        self.git('update-ref', 'refs/remotes/origin/release-v3.0', 'HEAD')
        self.assert_rejected('v3.0.0-rc01', error='already exists')
        self.git('tag', 'v4.1')
        self.assert_rejected('v4.0.0-rc01', error='marker tag')

    def test_stable_releases_from_release_branch(self):
        for kind, rc, version in (('minor', 'v2.13.0-rc03', 'v2.13.0'),
                                  ('major', 'v3.0.0-rc02', 'v3.0.0'),
                                  ('patch', 'v2.14.0', 'v2.14.1')):
            with self.subTest(kind=kind):
                self.series(rc)
                result, outputs = self.prepare(version, kind)
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                self.assertEqual(outputs['branch'], 'release-' + '.'.join(version.split('.')[:2]))
                self.assertEqual(outputs['type'], 'release')
                self.assertEqual(outputs['channel'], 'stable')
                self.assertNotIn('new-branch', outputs)
                self.write_version(version)
                self.git('tag', version)
                self.assert_rejected(version, kind, 'already exists')
                self.git('checkout', 'master')

    def test_stable_constraints(self):
        for kind, version in (('major', 'v3.1.0'), ('major', 'v3.0.1'),
                              ('minor', 'v2.13.1'), ('minor', 'v02.13.0'),
                              ('minor', 'v2.13.0-rc01'), ('bogus', 'v2.13.0')):
            with self.subTest(kind=kind, version=version):
                self.assert_rejected(version, kind)
        self.series('v2.13.2')
        self.git('checkout', 'release-v2.13')
        self.assert_rejected('v2.13.1', 'patch', 'not newer')

    def test_workflow_publishes_branch_marker_and_tags_atomically(self):
        self.git('tag', '-a', 'v2.13', '-m', 'development marker')
        remote = self.root / 'remote.git'
        self.git('init', '--bare', str(remote))
        self.git('remote', 'add', 'origin', str(remote))
        self.git('push', 'origin', 'master')
        (self.root / 'CHANGELOG.md').write_text('Synthetic changelog\n')
        workflow = (SCRIPT.parent.parent / 'workflows/release.yml').read_text()
        step = workflow.split('      - name: Commit Changes\n', 1)[1].split('      - name:', 1)[0]
        script = '\n'.join(line[10:] for line in step.split('        run: |\n', 1)[1].splitlines())
        remote_git = ('--git-dir', str(remote))
        for version, kind in (('v2.13.0-rc01', 'release-candidate'),
                              ('v2.13.0-rc03', 'release-candidate'), ('v2.13.0', 'minor')):
            with self.subTest(version=version):
                result, outputs = self.prepare(version, kind)
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                env = {**os.environ, 'RELEASE_MESSAGE': outputs['message'],
                       'RELEASE_TYPE': outputs['type'], 'RELEASE_TAG': outputs['ref'],
                       'RELEASE_BRANCH': outputs['branch'],
                       'RELEASE_NEW_BRANCH': outputs.get('new-branch', ''),
                       'RELEASE_MARKER': outputs.get('marker', ''),
                       'GITHUB_OUTPUT': str(self.root / 'commit-output')}
                subprocess.run(['bash', '-eu', '-c', script], cwd=self.root,
                               env=env, check=True, capture_output=True)
                self.assertEqual(self.git(*remote_git, 'show', f'{version}:packaging/version'), version)
                self.assertEqual(self.git(*remote_git, 'show', 'release-v2.13:packaging/version'), version)
                # Release tags never land on master.
                ancestor = subprocess.run(['git', *remote_git, 'merge-base', '--is-ancestor', version, 'master'],
                                          cwd=self.root)
                self.assertNotEqual(ancestor.returncode, 0)
        # The marker is an empty, annotated commit on master, on top of the branch point.
        self.assertEqual(self.git(*remote_git, 'describe', 'master'), 'v2.14')
        self.assertEqual(self.git(*remote_git, 'rev-parse', 'master^'),
                         self.git(*remote_git, 'merge-base', 'master', 'release-v2.13'))
        self.assertEqual(self.git(*remote_git, 'diff', '--stat', 'master^', 'master'), '')
        self.assertEqual(self.git(*remote_git, 'show', 'master:packaging/version'), 'v2.12.3-1-nightly')
        self.assert_rejected('v2.13.0-rc04', error='regular release')

    def release(self, version, kind):
        result, outputs = self.prepare(version, kind)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.write_version(version)
        self.git('tag', version)

    def test_release_sequence_cannot_move_backwards(self):
        self.series('v2.13.0-rc01')
        self.git('checkout', 'release-v2.13')
        # No patch before the series' first release, so neither an older RC nor GA can follow it.
        self.assert_rejected('v2.13.1', 'patch', 'requires a released')
        self.release('v2.13.0-rc02', 'release-candidate')
        self.release('v2.13.0', 'minor')
        self.assert_rejected('v2.13.0-rc03', error='regular release')
        self.release('v2.13.1', 'patch')
        self.assert_rejected('v2.13.0', 'minor', 'already exists')

    def test_rc_cannot_follow_a_patch_on_its_branch(self):
        # A branch can already be past GA without a matching GA tag (e.g. a tag deleted after a failed release).
        self.git('tag', 'v2.13.0-rc01')
        self.series('v2.13.1')
        self.git('checkout', 'release-v2.13')
        self.assert_rejected('v2.13.0-rc02', error='cannot follow')

    def test_branch_without_release_tooling(self):
        # A maintenance branch cut before this tooling existed (like v2.12 at v2.12.3). Preparation runs from master.
        self.git('checkout', '-b', 'release-v2.12')
        self.git('rm', '-q', '-r', '.github', 'packaging/release_info.py')
        self.write_version('v2.12.3')

        def rejected(missing):
            self.git('checkout', '-q', 'master')
            result, outputs = self.prepare('v2.12.4', 'patch')
            self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertEqual(outputs, {})
            for item in missing:
                self.assertIn(item, result.stdout)
            self.git('checkout', '-q', 'release-v2.12')

        rejected(['packaging/release_info.py', '.github/workflows/build.yml(input:source_sha)'])
        self.git('checkout', 'master', '--', 'packaging/release_info.py')
        self.git('commit', '-m', 'backport helper')
        rejected(['.github/workflows/docker.yml(input:companion_run_id)'])
        self.git('checkout', 'master', '--', '.github')
        self.git('commit', '-m', 'backport workflows')
        self.git('checkout', '-q', 'master')
        result, outputs = self.prepare('v2.12.4', 'patch')
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_windows_version_bounds(self):
        self.series('v2.13.0')
        self.git('checkout', 'release-v2.13')
        self.assert_rejected('v2.13.65436', 'patch', 'MSI limits')
        self.release('v2.13.65435', 'patch')
        self.git('checkout', 'master')
        for version in ('v256.0.0-rc01', 'v2.256.0-rc01'):
            with self.subTest(version=version):
                self.assert_rejected(version, error='MSI limits')
        for version in ('v255.0.0-rc01', 'v2.255.0-rc01'):
            with self.subTest(version=version):
                result, outputs = self.prepare(version)
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                self.git('checkout', '--', 'packaging/version')

    def test_nightly_windows_version_bounds(self):
        self.git('commit', '--allow-empty', '-m', 'release')
        self.git('tag', 'v2.12.65435')
        self.assertEqual(self.nightly(), 'v2.12.65435-0-post-nightly')
        self.git('checkout', '--', 'packaging/version')
        self.git('commit', '--allow-empty', '-m', 'change')
        self.git('tag', 'v2.12.65436')
        result, outputs = self.prepare('nightly', 'nightly')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('MSI limits', result.stdout)

    def nightly(self):
        result, outputs = self.prepare('nightly', 'nightly')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(outputs['type'], 'nightly')
        self.assertEqual(outputs['channel'], 'nightly')
        return (self.root / 'packaging/version').read_text().strip()

    def test_nightly_versions(self):
        # Before marker tags exist, nightlies follow the last stable tag.
        self.assertEqual(self.nightly(), 'v2.12.3-0-post-nightly')
        self.write_version('v2.12.3-0-post-nightly')
        result, outputs = self.prepare('nightly', 'nightly', event='schedule')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(outputs, {'run': 'false'})
        # A nightly of the marker commit itself is number 0.
        self.git('commit', '--allow-empty', '-m', 'marker')
        self.git('tag', '-a', 'v2.13', '-m', 'marker')
        self.assertEqual(self.nightly(), 'v2.13-0-nightly')
        self.write_version('v2.13-0-nightly')
        self.git('commit', '--allow-empty', '-m', 'change')
        self.assertEqual(self.nightly(), 'v2.13-2-nightly')
        # A stray prerelease tag on master would yield an unparseable nightly version.
        self.git('tag', '-a', 'v2.14.0-rc01', '-m', 'stray')
        result, outputs = self.prepare('nightly', 'nightly')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('not a stable or development marker tag', result.stdout)

    def test_fork_guard(self):
        result, outputs = self.prepare('v2.13.0-rc01', repo='example/fork')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(outputs, {'run': 'false'})


if __name__ == '__main__':
    unittest.main()

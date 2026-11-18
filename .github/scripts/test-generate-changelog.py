#!/usr/bin/env python3
"""Exercise changelog generation with real, divergent Git histories and git-cliff."""

import os
from pathlib import Path
import re
import shutil
import subprocess
import tempfile
import unittest

REPO = Path(__file__).resolve().parents[2]


@unittest.skipUnless(shutil.which('git-cliff'), 'requires git-cliff (installed by release-tests.yml)')
class ChangelogTest(unittest.TestCase):
    def setUp(self):
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        self.root = Path(directory.name)
        (self.root / 'packaging').mkdir()
        for source in (REPO / 'packaging').glob('generate-changelog.*'):
            shutil.copy2(source, self.root / 'packaging' / source.name)
        shutil.copy2(REPO / 'packaging/cliff.toml', self.root / 'packaging/cliff.toml')
        self.env = {**os.environ, 'GIT_AUTHOR_DATE': '2025-01-01T00:00:00Z',
                    'GIT_COMMITTER_DATE': '2025-01-01T00:00:00Z', 'TZ': 'UTC'}
        self.git('init', '-b', 'master')
        self.git('config', 'user.name', 'Changelog test')
        self.git('config', 'user.email', 'changelog@example.invalid')
        self.git('config', 'commit.gpgsign', 'false')
        self.git('config', 'tag.gpgsign', 'false')
        self.commit('Historical baseline', 'v1.47.0')
        self.tag('v1.47.0')
        self.commit('Initial release feature (#101)', 'v2.0.0')
        self.tag('v2.0.0')

    def git(self, *args):
        return subprocess.run(['git', *args], cwd=self.root, env=self.env,
                              capture_output=True, text=True, check=True).stdout.strip()

    def commit(self, message, version=None):
        if version is not None:
            (self.root / 'packaging/version').write_text(version + '\n')
            self.git('add', 'packaging/version')
        self.git('commit', '--allow-empty', '-m', message)

    def tag(self, name):
        self.git('tag', '-a', name, '-m', name)

    def generate(self):
        result = subprocess.run(['sh', 'packaging/generate-changelog.sh'], cwd=self.root,
                                env=self.env, capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        text = (self.root / 'CHANGELOG.md').read_text()
        headers = list(re.finditer(r'^## \[([^]]+)\].*$', text, re.M))
        return {match.group(1): text[match.end():headers[index + 1].start() if index + 1 < len(headers) else None]
                for index, match in enumerate(headers)}, text

    def split_series(self):
        self.commit('[ci skip] Start development')
        self.tag('v2.1')
        self.commit('Shared feature for 2.1 (#102)', 'v2.1-1-nightly')
        self.git('checkout', '-b', 'release-v2.1')
        self.commit('[ci skip] RC preparation', 'v2.1.0-rc01')
        self.tag('v2.1.0-rc01')
        self.commit('Release branch fix (#103)')
        self.commit('[ci skip] Final preparation', 'v2.1.0')
        self.env['GIT_COMMITTER_DATE'] = '2025-02-03T04:05:06Z'
        self.tag('v2.1.0')
        self.git('checkout', 'master')
        self.commit('[ci skip] Open next series')
        self.tag('v2.2')
        self.commit('Next master feature (#104)', 'v2.2-1-nightly')

    def test_divergent_release_history(self):
        self.split_series()
        sections, text = self.generate()
        self.assertEqual(list(sections), ['unreleased', '2.1.0', '2.0.0'])
        self.assertIn('Next master feature', sections['unreleased'])
        self.assertNotIn('Shared feature', sections['unreleased'])
        self.assertNotIn('Release branch fix', sections['unreleased'])
        self.assertIn('Shared feature', sections['2.1.0'])
        self.assertIn('Release branch fix', sections['2.1.0'])
        self.assertIn('Initial release feature', sections['2.0.0'])
        self.assertIn('## [2.1.0] - 2025-02-03', text)
        self.assertIn('[#102](https://github.com/netdata/netdata/issues/102)', text)
        self.assertNotIn('[ci skip]', text)

    def test_late_maintenance_patch_and_maintenance_baseline(self):
        self.split_series()
        self.git('checkout', '-b', 'release-v2.0', 'v2.0.0')
        self.commit('Late maintenance fix (#105)', 'v2.0.1')
        self.tag('v2.0.1')
        self.commit('Maintenance work awaiting release (#106)', 'v2.0.2')
        sections, _ = self.generate()
        self.assertEqual(list(sections), ['unreleased', '2.1.0', '2.0.1', '2.0.0'])
        self.assertIn('Maintenance work awaiting release', sections['unreleased'])
        self.assertNotIn('Initial release feature', sections['unreleased'])
        self.assertIn('Late maintenance fix', sections['2.0.1'])
        self.assertNotIn('Shared feature', sections['2.0.1'])
        self.git('checkout', 'master')
        master_sections, _ = self.generate()
        self.assertIn('Next master feature', master_sections['unreleased'])
        self.assertNotIn('Shared feature', master_sections['unreleased'])
        self.assertIn('Late maintenance fix', master_sections['2.0.1'])

    def test_prerelease_and_nightly_remain_unreleased(self):
        for name, version in {'RC': 'v2.1.0-rc02', 'nightly': 'v2.1-9-nightly',
                              'post nightly': 'v2.0.0-9-post-nightly'}.items():
            with self.subTest(name=name):
                self.commit('Work awaiting a stable release', version)
                if name == 'RC':
                    self.tag('v2.1.0-rc02')
                sections, _ = self.generate()
                self.assertEqual(list(sections), ['unreleased', '2.0.0'])
                self.assertIn('Work awaiting a stable release', sections['unreleased'])

    def test_numeric_order_and_empty_releases(self):
        self.commit('Feature in minor nine', 'v2.9.0')
        self.tag('v2.9.0')
        self.commit('Feature in minor ten', 'v2.10.0')
        self.tag('v2.10.0')
        self.commit('[ci skip] Empty patch', 'v2.10.1')
        self.tag('v2.10.1')
        sections, text = self.generate()
        self.assertEqual(list(sections), ['2.10.1', '2.10.0', '2.9.0', '2.0.0'])
        self.assertNotIn('unreleased', text)
        self.assertIn('Feature in minor ten', sections['2.10.0'])
        self.assertNotIn('Feature in minor nine', sections['2.10.0'])

    def test_unreleased_does_not_depend_on_stale_version_at_marker(self):
        self.split_series()
        self.commit('[ci skip] Stale packaging version on marker', 'v2.0.0-50-nightly')
        sections, _ = self.generate()
        self.assertNotIn('Shared feature', sections['unreleased'])
        self.assertIn('Next master feature', sections['unreleased'])

    def test_failure_preserves_existing_changelog(self):
        (self.root / 'CHANGELOG.md').write_text('Existing release history\n')
        # A shallow/incomplete checkout must fail rather than silently truncate history.
        self.git('update-ref', '-d', 'refs/tags/v1.47.0')
        result = subprocess.run(['sh', 'packaging/generate-changelog.sh'], cwd=self.root,
                                env=self.env, capture_output=True, text=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual((self.root / 'CHANGELOG.md').read_text(), 'Existing release history\n')

    def test_render_failure_preserves_existing_changelog(self):
        (self.root / 'CHANGELOG.md').write_text('Existing release history\n')
        self.commit('New work', 'v2.1-1-nightly')
        config = self.root / 'packaging/cliff.toml'
        # Malformed Tera makes the real renderer fail after Git inventory selection succeeds.
        config.write_text(config.read_text().replace('{% if version %}', '{% if %}'))
        result = subprocess.run(['sh', 'packaging/generate-changelog.sh'], cwd=self.root,
                                env=self.env, capture_output=True, text=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual((self.root / 'CHANGELOG.md').read_text(), 'Existing release history\n')

    def test_lightweight_tag_date_and_noncanonical_exclusion(self):
        self.env['GIT_COMMITTER_DATE'] = '2025-03-04T05:06:07Z'
        self.commit('Lightweight release feature', 'v2.1.0')
        self.git('tag', 'v2.1.0')
        self.tag('v02.1.0')
        self.tag('v2.1.0-rc03')
        self.tag('v2.1')
        sections, text = self.generate()
        self.assertEqual(list(sections), ['2.1.0', '2.0.0'])
        self.assertIn('## [2.1.0] - 2025-03-04', text)


if __name__ == '__main__':
    unittest.main()

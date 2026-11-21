#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later

import json
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

from ruamel.yaml import YAML


ROOT = Path(__file__).resolve().parents[3]
SCRIPTS = ROOT / '.github/scripts'


class DockerVariantsTest(unittest.TestCase):
    def test_release_or_dem_changes_select_the_complete_image(self):
        with (ROOT / '.github/workflows/docker.yml').open() as stream:
            file_check = YAML(typ='safe').load(stream)['jobs']['file-check']
        script = next(step['run'] for step in file_check['steps'] if step.get('id') == 'check-dem')
        for release, changed, selected in (
            ('true', '', 'true'),
            ('false', '', 'false'),
            ('false', 'false', 'false'),
            ('false', 'true', 'true'),
        ):
            with self.subTest(release=release, changed=changed), tempfile.TemporaryDirectory() as directory:
                output = Path(directory) / 'output'
                subprocess.run(['bash', '-e', '-c', script], check=True,
                               env=dict(os.environ, RELEASE=release, CHANGED=changed, GITHUB_OUTPUT=str(output)))
                self.assertEqual(output.read_text().strip(), f'dem={selected}')

    def test_build_and_publication_matrices(self):
        regular = ('amd64', 'armv7l', 'arm64')
        for include_dem in ('false', 'true'):
            for native_only in ('0', '1'):
                with self.subTest(include_dem=include_dem, native_only=native_only):
                    result = subprocess.run(
                        [sys.executable, SCRIPTS / 'gen-matrix-docker.py', native_only, include_dem],
                        cwd=ROOT, check=True, capture_output=True, text=True,
                    )
                    matrix = json.loads(result.stdout)
                    expected = {'regular': regular}
                    if include_dem == 'true':
                        expected['dem'] = ('amd64', 'arm64')
                    actual = {row['variant']: tuple(row['arches'].split()) for row in matrix['publish']['include']}
                    self.assertEqual(expected, actual)
                    self.assertEqual([(variant, arch) for variant, arches in expected.items() for arch in arches],
                                     [(row['variant'], row['arch']) for row in matrix['build']['include']])
                    self.assertEqual([(arch, arch in expected.get('dem', ())) for arch in regular],
                                     [(row['arch'], row['dem']) for row in matrix['test']['include']])
                    for row in matrix['build']['include']:
                        if row['variant'] == 'dem':
                            self.assertFalse(row['qemu'])
                            self.assertTrue(row['platform'].startswith('linux/' + row['arch']))

    def manifest(self, files, arches, repo, tags):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            for arch, digest in files.items():
                (root / arch).write_text(digest + '\n')
            return subprocess.run(
                [sys.executable, SCRIPTS / 'gen-docker-imagetool-args.py', root, repo, tags, *arches],
                cwd=ROOT, capture_output=True, text=True,
            )

    def test_manifest_only_admits_the_variant_architectures_and_registry_tags(self):
        files = {'amd64': 'sha256:' + 'a' * 64, 'arm64': 'sha256:' + 'b' * 64}
        arches = ('amd64', 'arm64')
        repo = 'quay.io/netdata/netdata'
        tags = 'netdata/netdata:edge-dem,quay.io/netdata/netdata:edge-dem,ghcr.io/netdata/netdata:edge-dem'
        manifest = self.manifest(files, arches, repo, tags)
        self.assertEqual(manifest.returncode, 0, manifest.stderr)
        self.assertEqual(manifest.stdout.split(), ['-t', repo + ':edge-dem'] + [
            f'{repo}@{files[arch]}' for arch in arches
        ])
        for bad in (
            {'arm64': files['arm64']},
            dict(files, armv7l='sha256:' + 'c' * 64),
            dict(files, amd64=files['arm64']),
            dict(files, amd64='wrong'),
        ):
            with self.subTest(files=bad):
                manifest = self.manifest(bad, arches, repo, tags)
                self.assertNotEqual(manifest.returncode, 0)
                self.assertEqual(manifest.stdout, '')

    def test_dem_nightly_rotation_uses_its_own_tags(self):
        with tempfile.TemporaryDirectory() as directory:
            docker = Path(directory) / 'docker'
            log = Path(directory) / 'log'
            docker.write_text('#!/bin/sh\nprintf "%s\\n" "$*" >> "$DOCKER_TEST_LOG"\n')
            docker.chmod(0o755)
            repo = 'ghcr.io/netdata/netdata'
            subprocess.run([SCRIPTS / 'rotate-docker-tags.sh', repo, 'edge-dem', '2'], check=True,
                           env=dict(os.environ, PATH=directory + os.pathsep + os.environ['PATH'],
                                    DOCKER_TEST_LOG=str(log)))
            self.assertEqual(log.read_text().splitlines(), [
                f'manifest inspect {repo}:edge-dem-1',
                f'buildx imagetools create --tag {repo}:edge-dem-2 {repo}:edge-dem-1',
                f'manifest inspect {repo}:edge-dem-0',
                f'buildx imagetools create --tag {repo}:edge-dem-1 {repo}:edge-dem-0',
                f'manifest inspect {repo}:edge-dem',
                f'buildx imagetools create --tag {repo}:edge-dem-0 {repo}:edge-dem',
            ])


if __name__ == '__main__':
    unittest.main()

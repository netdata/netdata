#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later

import json
import os
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


SCRIPT = Path(__file__).resolve().parent / 'get-go-version.py'
BRIDGE = 'src/web/mcp/bridges/stdio-golang'
MODULE_VERSIONS = {
    'src/go': '1.27.0',
    BRIDGE: '1.24.0',
    'src/collectors/cgroups.plugin/cgroup-name': '1.26.0',
    'src/collectors/ebpf.plugin/ebpfgo.plugin': '1.25.0',
    'src/health/notifications/alarm-notify': '1.23.0',
}
PLATFORMS = {
    'linux/386', 'linux/amd64', 'linux/arm', 'linux/arm64', 'linux/ppc64le',
    'windows/amd64', 'darwin/amd64', 'darwin/arm64', 'freebsd/amd64',
}
GO_TARGETS = {
    './cmd/godplugin', './cmd/ipmiplugin', './tools/topology-ip-intel-downloader', './tools/synthetic',
}


class GoBuildMatrixTest(unittest.TestCase):
    def setUp(self):
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        root = Path(directory.name)
        script = root / '.github/scripts/get-go-version.py'
        script.parent.mkdir(parents=True)
        shutil.copyfile(SCRIPT, script)

        for module, version in MODULE_VERSIONS.items():
            path = root / module
            path.mkdir(parents=True)
            (path / 'go.mod').write_text(f'module example.invalid/{path.name}\n\ngo {version}\n', encoding='utf-8')
            if module != 'src/go':
                (path / 'bridge.go').write_text('package main\nfunc main() {}\n', encoding='utf-8')

        for target in GO_TARGETS | {'./examples/demo', './cmd/ibmdplugin'}:
            path = root / 'src/go' / target
            path.mkdir(parents=True)
            (path / 'main.go').write_text('package main\nfunc main() {}\n', encoding='utf-8')

        output = root / 'github-output'
        result = subprocess.run(
            [sys.executable, str(script)], env=dict(os.environ, GITHUB_OUTPUT=str(output)),
            text=True, capture_output=True,
        )
        self.assertEqual(result.returncode, 0, result.stderr)
        self.matrices = {
            key: json.loads(value)
            for key, value in (line.split('=', 1) for line in output.read_text(encoding='utf-8').splitlines())
        }

    def test_native_tests_cover_each_module_once(self):
        expected = [
            {
                'module': module,
                'version': version,
                'targets': ' '.join(sorted(GO_TARGETS)) if module == 'src/go' else '.',
            }
            for module, version in MODULE_VERSIONS.items()
        ]
        self.assertCountEqual(self.matrices['matrix'], expected)

    def test_cross_builds_cover_each_module_and_platform_once(self):
        entries = self.matrices['build_matrix']
        expected_keys = [(module, platform) for module in ('src/go', BRIDGE) for platform in PLATFORMS]
        self.assertCountEqual([(entry['module'], entry['platform']) for entry in entries], expected_keys)

        for entry in entries:
            with self.subTest(module=entry['module'], platform=entry['platform']):
                goos, goarch = entry['platform'].split('/')
                self.assertEqual(entry, {
                    'module': entry['module'],
                    'platform': entry['platform'],
                    'goos': goos,
                    'goarch': goarch,
                    'version': MODULE_VERSIONS[entry['module']],
                    'targets': entry['targets'],
                })
                self.assertTrue(entry['targets'])

    def test_cross_build_targets_match_supported_platforms(self):
        for entry in self.matrices['build_matrix']:
            with self.subTest(module=entry['module'], platform=entry['platform']):
                if entry['module'] == BRIDGE:
                    expected = {'.'}
                else:
                    expected = {'./cmd/godplugin', './tools/synthetic'}
                    if entry['goos'] == 'linux':
                        expected.add('./cmd/ipmiplugin')
                    if entry['platform'] not in {'linux/386', 'linux/arm', 'windows/amd64'}:
                        expected.add('./tools/topology-ip-intel-downloader')

                self.assertCountEqual(entry['targets'].split(), expected)


if __name__ == '__main__':
    unittest.main()

#!/usr/bin/env python3

import json
import os
import pathlib

SCRIPT_PATH = pathlib.Path(__file__).parents[0]
REPO_ROOT = SCRIPT_PATH.parents[1]
GO_SRC = REPO_ROOT / 'src' / 'go'

GITHUB_OUTPUT = pathlib.Path(os.environ['GITHUB_OUTPUT'])

# Platforms the Go programs are cross-compiled for.
PLATFORMS = [
    'linux/386',
    'linux/amd64',
    'linux/arm',
    'linux/arm64',
    'linux/ppc64le',
    'windows/amd64',
    'darwin/amd64',
    'darwin/arm64',
    'freebsd/amd64',
]

# Programs (repository-relative directories) skipped on some platforms, mirroring where CMake builds them.
UNSUPPORTED_PLATFORMS = {
    # Linux only: ENABLE_PLUGIN_IPMI requires OS_LINUX (packaging/cmake/Modules/NetdataOptions.cmake).
    'src/go/cmd/ipmiplugin': [platform for platform in PLATFORMS if not platform.startswith('linux/')],
    # Not built on 32-bit targets (packaging/cmake/Modules/NetdataPluginNetflow.cmake), or on Windows where
    # ENABLE_PLUGIN_NETFLOW is disabled (packaging/cmake/Modules/NetdataOptions.cmake).
    'src/go/tools/topology-ip-intel-downloader': ['linux/386', 'linux/arm', 'windows/amd64'],
}

# Standalone Go modules outside src/go that CMake builds on every platform: tested natively and cross-compiled.
CROSS_BUILD_MODULES = [
    REPO_ROOT / 'src' / 'web' / 'mcp' / 'bridges' / 'stdio-golang',
]

# Standalone Go modules outside src/go that are Linux-only or developer tools: native Linux tests only.
EXTRA_TEST_MODULES = [
    REPO_ROOT / 'src' / 'collectors' / 'cgroups.plugin' / 'cgroup-name',
    REPO_ROOT / 'src' / 'collectors' / 'ebpf.plugin' / 'ebpfgo.plugin',
    REPO_ROOT / 'src' / 'health' / 'notifications' / 'alarm-notify',
]


def go_version(modfile):
    for line in modfile.read_text().splitlines():
        if line.startswith('go '):
            return line.split()[1]

    raise SystemExit(f'{modfile}: no go directive')


# One entry per module for the native Linux tests.
matrix = []
# One entry per cross-built module and platform for the cross-compilation tests.
build_matrix = []


def add_module(moddir, version, targets):
    """Add the native test entry and the per-platform build entries of a module; targets are go build packages."""
    modpath = moddir.relative_to(REPO_ROOT).as_posix()

    matrix.append({
        'module': modpath,
        'version': version,
        'targets': ' '.join(targets),
    })

    for platform in PLATFORMS:
        goos, goarch = platform.split('/')
        # pathlib maps the root package "." to the module directory itself.
        supported = [
            target for target in targets
            if platform not in UNSUPPORTED_PLATFORMS.get((moddir / target).relative_to(REPO_ROOT).as_posix(), [])
        ]

        build_matrix.append({
            'platform': platform,
            'goos': goos,
            'goarch': goarch,
            'module': modpath,
            'version': version,
            'targets': ' '.join(supported),
        })


for modfile in sorted(GO_SRC.glob('**/go.mod')):
    moddir = modfile.parent
    targets = []

    for main in moddir.glob('**/main.go'):
        mainpath = main.relative_to(moddir).parent

        if 'examples' in mainpath.parts:
            continue

        # Skip ibmdplugin as it requires CGO
        if 'ibmdplugin' in mainpath.parts:
            continue

        targets.append(f'./{mainpath.as_posix()}')

    targets.sort()
    add_module(moddir, go_version(modfile), targets)

for moddir in CROSS_BUILD_MODULES:
    modfile = moddir / 'go.mod'
    if not modfile.exists():
        continue

    add_module(moddir, go_version(modfile), ['.'])

for moddir in EXTRA_TEST_MODULES:
    modfile = moddir / 'go.mod'
    if not modfile.exists():
        continue

    matrix.append({
        'module': moddir.relative_to(REPO_ROOT).as_posix(),
        'version': go_version(modfile),
        'targets': '.',
    })

with GITHUB_OUTPUT.open('a') as f:
    f.write(f'matrix={json.dumps(matrix)}\n')
    f.write(f'build_matrix={json.dumps(build_matrix)}\n')

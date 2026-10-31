#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
"""Shared CI runner selection and native repository submission."""
import argparse
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile

from bundle import run


ROOT = Path(__file__).resolve().parents[2]
ARCHES = ('amd64', 'arm64')
PACKAGE_ARCHES = {'deb': {'amd64': 'amd64', 'arm64': 'arm64'},
                  'rpm': {'x86_64': 'amd64', 'aarch64': 'arm64'}}
PACKAGE_HOSTS = ('packages.netdata.cloud', 'packages2.netdata.cloud')
PACKAGE_REPOSITORIES = ('netdata/netdata', 'netdata/netdata-edge', 'netdata/netdata-devel')


def read_catalog(path):
    from ruamel.yaml import YAML
    with path.open() as stream:
        return YAML(typ='safe').load(stream)


def build_matrix(catalog):
    return {'include': [{'arch': arch, 'runner': catalog['arch_data'][arch]['runner']}
                        for arch in ARCHES]}


def package_targets(catalog):
    targets = set()
    for distro in catalog['include']:
        package = distro.get('packages')
        if not package:
            continue
        fmt = package['type']
        if fmt not in PACKAGE_ARCHES:
            raise ValueError('unsupported native package format: ' + fmt)
        for arch in package['arches']:
            runtime_arch = PACKAGE_ARCHES[fmt].get(arch)
            if runtime_arch:
                targets.add((package['repo_distro'], fmt, arch, runtime_arch))
    if not targets:
        raise ValueError('catalog has no supported native runtime destinations')
    return sorted(targets)


def upload_native(catalog, artifacts):
    targets = package_targets(catalog)
    # Check all source packages before submitting any destination. The shared
    # uploader recursively selects files by extension from this directory.
    for _, fmt, _, runtime_arch in targets:
        source = artifacts / ('dem-runtime-' + runtime_arch)
        if len(list(source.rglob('*.' + fmt))) != 1:
            raise ValueError('expected one .' + fmt + ' package in ' + str(source))
    for host in PACKAGE_HOSTS:
        for repository in PACKAGE_REPOSITORIES:
            for distro, fmt, arch, runtime_arch in targets:
                # package-upload.sh stages below TMPDIR; isolate each call so
                # earlier distro/architecture payloads cannot leak into it.
                with tempfile.TemporaryDirectory(prefix='dem-runtime-upload-') as temporary:
                    env = dict(os.environ, TMPDIR=temporary)
                    run(['sh', ROOT / '.github/scripts/package-upload.sh', host, distro,
                         arch, fmt, repository, artifacts / ('dem-runtime-' + runtime_arch)], env=env)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--distros', type=Path, default=ROOT / '.github/data/distros.yml')
    commands = parser.add_subparsers(dest='command', required=True)
    commands.add_parser('build-matrix')
    upload = commands.add_parser('upload-native')
    upload.add_argument('--artifacts', required=True, type=Path)
    args = parser.parse_args()
    catalog = read_catalog(args.distros)
    if args.command == 'build-matrix':
        print(json.dumps(build_matrix(catalog), sort_keys=True))
    else:
        upload_native(catalog, args.artifacts.resolve())


if __name__ == '__main__':
    try:
        main()
    except subprocess.CalledProcessError as error:
        sys.exit(error.returncode if error.returncode > 0 else 1)

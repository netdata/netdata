#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
"""Select runtime build runners from the Agent's shared distro catalog."""
import argparse
import json
import subprocess
import sys
from pathlib import Path
import tarfile
import tempfile

from bundle import read_release_info, run, validate_release_info
from release import empty_directory, validate_inventory


ROOT = Path(__file__).resolve().parents[2]
ARCHES = ('amd64', 'arm64')


def read_catalog(path):
    from ruamel.yaml import YAML
    with path.open() as stream:
        return YAML(typ='safe').load(stream)


def build_matrix(catalog):
    return {'include': [{'arch': arch, 'runner': catalog['arch_data'][arch]['runner']}
                        for arch in ARCHES]}


def verify_publication_receipt(args):
    """Check the small receipt from a proven successful parent publication job.

    This binds lane inputs to that job; only the publisher and native consumers
    validate payload bytes. The workflow separately checks GitHub run provenance.
    """
    expected = read_release_info(args.release_info)
    if read_release_info(args.receipt / 'release-info') != expected:
        raise ValueError('publication receipt belongs to another Agent release')
    records = {}
    for arch in ARCHES:
        record = json.loads((args.receipt / (arch + '.json')).read_text())
        if (record['format'] != 2 or record['architecture'] != arch or
                validate_release_info(record['agent']) != expected):
            raise ValueError('publication receipt architecture or Agent identity differs')
        records[arch] = record
    return records


def prepare_native(args):
    """Admit one exact parent-run artifact for the existing Agent package build."""
    expected = read_release_info(args.release_info)
    inventory = validate_inventory(args.artifacts, args.arch)
    if inventory['agent'] != expected:
        raise ValueError('native artifact belongs to another Agent release')
    if inventory != verify_publication_receipt(args)[args.arch]:
        raise ValueError('native artifact differs from the successful publication receipt')
    output = empty_directory(args.output)
    with tempfile.TemporaryDirectory(prefix='dem-native-extract-') as temporary:
        archive = Path(temporary) / 'payload.tar'
        run(['zstd', '-d', '--no-progress', '-o', archive, args.artifacts / 'native-payload.tar.zst'])
        with tarfile.open(archive) as payload:
            payload.extractall(output, filter='data')
    if (output / 'release-info').read_bytes() != args.release_info.read_bytes():
        raise ValueError('native payload release-info differs from the Agent source')
    native = json.loads((output / 'native.json').read_text())
    manifest = json.loads((args.artifacts / 'manifest.json').read_text())
    if (native['format'] != 1 or native['path'] != '/usr/lib/netdata/dem-runtime' or
            native['manifest'] != manifest or manifest['platform'] != 'linux/' + args.arch or
            manifest['version'] != expected['version']):
        raise ValueError('native payload metadata differs from the exact Agent companion')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--distros', type=Path, default=ROOT / '.github/data/distros.yml')
    commands = parser.add_subparsers(dest='command', required=True)
    commands.add_parser('build-matrix')
    receipt = commands.add_parser('verify-publication-receipt')
    receipt.add_argument('--receipt', type=Path, required=True)
    receipt.add_argument('--release-info', type=Path, required=True)
    native = commands.add_parser('prepare-native')
    native.add_argument('--artifacts', type=Path, required=True,
                        help='downloaded exact-architecture artifact directory')
    native.add_argument('--arch', choices=ARCHES, required=True)
    native.add_argument('--release-info', type=Path, required=True)
    native.add_argument('--receipt', type=Path, required=True,
                        help='successful publication receipt from the same parent run')
    native.add_argument('--output', type=Path, required=True, help='new or empty prebound payload directory')
    args = parser.parse_args()
    if args.command == 'prepare-native':
        prepare_native(args)
    elif args.command == 'verify-publication-receipt':
        verify_publication_receipt(args)
    else:
        print(json.dumps(build_matrix(read_catalog(args.distros)), sort_keys=True))


if __name__ == '__main__':
    try:
        main()
    except subprocess.CalledProcessError as error:
        sys.exit(error.returncode if error.returncode > 0 else 1)
    except ValueError as error:
        print('runtime artifact operation failed: ' + str(error), file=sys.stderr)
        sys.exit(1)

#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
"""Build the DEM runtime in an immutable disposable donor; no host installs."""
import argparse
import json
import os
from pathlib import Path
import subprocess
import sys
import shutil
import tempfile

from bundle import run, validate_url_prefix
from locked import download


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--arch', required=True, choices=('amd64', 'arm64'))
    parser.add_argument('--output', required=True, type=Path)
    parser.add_argument('--cache', type=Path, help='reusable hash-verified binary input cache')
    parser.add_argument('--url-prefix', default='', help='approved HTTPS base; empty creates an offline-only manifest')
    args = parser.parse_args()
    try:
        validate_url_prefix(args.url_prefix)
    except ValueError as error:
        parser.error(str(error))
    recipe = Path(__file__).resolve().parent
    repo = recipe.parent.parent
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=True)
    if any(output.iterdir()):
        parser.error('--output must be empty (artifacts are never overwritten)')
    inputs = json.loads((recipe / 'inputs.json').read_text())
    arch = inputs['architectures'][args.arch]
    lock = json.loads((recipe / 'locks' / (args.arch + '.json')).read_text())
    donor = inputs['base'].split('@')[0] + '@' + arch['base_manifest']
    if lock['base_image'] != donor or lock['architecture'] != args.arch:
        raise ValueError('Debian lock differs from selected immutable base')
    cache = (args.cache or output.parent / 'binary-inputs').resolve()
    with tempfile.TemporaryDirectory(prefix='dem-runtime-build-') as temporary:
        context = Path(temporary)
        debs = context / 'debs'
        debs.mkdir()
        for record in lock['binary_packages']:
            record = dict(record, name=record['file'])
            source = download(record, cache / record['sha256'])
            shutil.copyfile(source, debs / record['name'])
        shutil.copyfile(recipe / 'Dockerfile', context / 'Dockerfile')
        image_id = context / 'image-id'
        run(['docker', 'build', '--platform', 'linux/' + args.arch, '--build-arg', 'BASE=' + donor,
             '--iidfile', image_id, context])
        donor = image_id.read_text().strip()
    # Only these read-only sources and the requested output are visible to the build.
    command = ['docker', 'run', '--rm', '--platform', 'linux/' + args.arch,
               '--cap-drop=ALL', '--cap-add=CHOWN', '--cap-add=DAC_OVERRIDE',
               '--security-opt=no-new-privileges', '--shm-size=1g',
               '--mount', f'type=bind,src={recipe},dst=/recipe,readonly',
               '--mount', f'type=bind,src={repo / "src/dem-runtime"},dst=/assets,readonly',
               '--mount', f'type=bind,src={output},dst=/out',
               '-e', f'OUTPUT_UID={os.getuid()}', '-e', f'OUTPUT_GID={os.getgid()}',
               donor, 'python3', '/recipe/bundle.py', '--arch', args.arch,
               '--url-prefix', args.url_prefix]
    run(command)


if __name__ == '__main__':
    try:
        main()
    except subprocess.CalledProcessError as error:
        sys.exit(error.returncode if error.returncode > 0 else 1)

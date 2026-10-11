#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
"""Build the DEM runtime in an immutable disposable donor; no host installs."""
import argparse
import json
import os
from pathlib import Path
import subprocess
import sys
from urllib.parse import urlsplit

from bundle import run


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--arch', required=True, choices=('amd64', 'arm64'))
    parser.add_argument('--output', required=True, type=Path)
    parser.add_argument('--url-prefix', default='', help='approved HTTPS base; empty creates an offline-only manifest')
    args = parser.parse_args()
    if args.url_prefix:
        url = urlsplit(args.url_prefix)
        if url.scheme != 'https' or not url.netloc or url.username or url.password or url.query or url.fragment:
            parser.error('--url-prefix must be an HTTPS base without credentials, query or fragment')
    recipe = Path(__file__).resolve().parent
    repo = recipe.parent.parent
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=True)
    if any(output.iterdir()):
        parser.error('--output must be empty (artifacts are never overwritten)')
    inputs = json.loads((recipe / 'inputs.json').read_text())
    arch = inputs['architectures'][args.arch]
    donor = inputs['donor'].split('@')[0] + '@' + arch['donor_manifest']
    # Only these read-only sources and the requested output are visible to the build.
    command = ['docker', 'run', '--rm', '--platform', 'linux/' + args.arch,
               '--cap-drop=ALL', '--cap-add=CHOWN', '--security-opt=no-new-privileges', '--shm-size=1g',
               '--mount', f'type=bind,src={recipe},dst=/recipe,readonly',
               '--mount', f'type=bind,src={repo / "src/go/plugin/dem/synthetic/runner/assets"},dst=/assets,readonly',
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

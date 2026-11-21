#!/usr/bin/env python3

from __future__ import annotations

import os
import sys

from typing import Final

from modules.release_versions import stable_reaches_preview, version_core

version: Final = sys.argv[2]

REPO: Final = 'netdata/netdata'
REPOS: Final = {
    'docker': REPO,
    'quay': f'quay.io/{REPO}',
    'ghcr': f'ghcr.io/{REPO}',
}

NIGHTLY_TAG: Final = 'edge'


match version:
    case '':
        versions = ['test']
    case 'nightly':
        versions = [NIGHTLY_TAG, 'latest']
    case _:
        version_core(version)
        if '-' in version.split('+', 1)[0]:
            versions = [f'v{version}']
        else:
            v = f'v{version}'.split('.')
            versions = [
                v[0],
                '.'.join(v[0:2]),
                '.'.join(v[0:3]),
                'stable',
            ]

            if stable_reaches_preview(version):
                versions.append('preview')

def write_output(name: str, value: str) -> None:
    with open(os.getenv('GITHUB_OUTPUT'), 'a') as f:
        f.write(f'{name}={value}\n')


write_output('nightly', '1' if version == 'nightly' else '0')
for suffix, prefix in (('', ''), ('-dem', 'dem_')):
    write_output(f'{prefix}nightly_tag', NIGHTLY_TAG + suffix)
    write_output(f'{prefix}tags', ','.join(
        f'{repo}:{tag}{suffix}' for repo in REPOS.values() for tag in versions
    ))

for registry, repo in REPOS.items():
    write_output(f'{registry}_tags', ','.join(f'{repo}:{tag}' for tag in versions))
    write_output(f'{registry}_repo', repo)

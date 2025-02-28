#!/usr/bin/env python3

from __future__ import annotations

import os
import sys

from typing import Final

github_event: Final = sys.argv[1]
version: Final = sys.argv[2]

REPO: Final = 'netdata/netdata'
REPOS: Final = {
    'docker': REPO,
    'quay': f'quay.io/{REPO}',
    'ghcr': f'ghcr.io/{REPO}',
}

REPO: Final = 'netdata/netdata'
QUAY_REPO: Final = f'quay.io/{REPO}'
GHCR_REPO: Final = f'ghcr.io/{REPO}'
NIGHTLY_TAG: Final = 'edge'

tags = {
    k: [] for k in REPOS.keys()
}

nightly = False

match version:
    case '':
        for h in REPOS.keys():
            tags[h] = [f'{REPOS[h]}:test']
    case 'nightly':
        for h in REPOS.keys():
            tags[h] = [f'{REPOS[h]}:{t}' for t in (NIGHTLY_TAG, 'latest')]

        nightly = True
    case _:
        v = f'v{version}'.split('.')

        versions: Final = (
            v[0],
            '.'.join(v[0:2]),
            '.'.join(v[0:3]),
            'stable',
        )

        for h in REPOS.keys():
            tags[h] = [f'{REPOS[h]}:{t}' for t in versions]

all_tags = [x for y in tags.values() for x in y]


def write_output(name: str, value: str) -> None:
    with open(os.getenv('GITHUB_OUTPUT'), 'a') as f:
        f.write(f'{name}={value}')


write_output('tags', ','.join(all_tags))
write_output('nightly', '1' if nightly else '0')
write_output('nightly_tag', NIGHTLY_TAG)

for h in REPOS.keys():
    write_output(f'{h}_tags', ','.join(tags[h]))
    write_output(f'{h}_repo', REPOS[h])

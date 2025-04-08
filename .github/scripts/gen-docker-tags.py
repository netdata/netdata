#!/usr/bin/env python3

import sys

github_event = sys.argv[1]
version = sys.argv[2]

REPO = 'netdata/netdata'

REPOS = (
    REPO,
    f'quay.io/{REPO}',
    f'ghcr.io/{REPO}',
)

match version:
    case '':
        tags = (f'{REPO}:test',)
    case 'nightly':
        tags = tuple([
            f'{r}:{t}' for r in REPOS for t in ('edge', 'latest')
        ])
    case _:
        v = f'v{version}'.split('.')

        tags = tuple([
            f'{r}:{t}' for r in REPOS for t in (
                v[0],
                '.'.join(v[0:2]),
                '.'.join(v[0:3]),
            )
        ])

        tags = tags + tuple([f'{r}:stable' for r in REPOS])

print(','.join(tags))

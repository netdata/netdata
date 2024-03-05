#!/usr/bin/env python3

import sys

github_event = sys.argv[1]
version = sys.argv[2]

REPO = 'netdata/netdata-ci-test'

REPOS = (
    REPO,
    # f'ghcr.io/{REPO}',
    f'quay.io/{REPO}',
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

print(','.join(tags))

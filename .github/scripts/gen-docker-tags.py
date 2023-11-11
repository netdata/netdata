#!/usr/bin/env python3

import sys

version = sys.argv[1].split('.')
suffix = sys.argv[2]

REPO = f'netdata/netdata{suffix}'
GHCR = f'ghcr.io/{REPO}'
QUAY = f'quay.io/{REPO}'

tags = []

for repo in [REPO, GHCR, QUAY]:
    tags.extend(
        (
            ':'.join([repo, version[0]]),
            ':'.join([repo, '.'.join(version[:2])]),
            ':'.join([repo, '.'.join(version[:3])]),
        )
    )
print(','.join(tags))

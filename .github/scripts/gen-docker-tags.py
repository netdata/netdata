#!/usr/bin/env python3

import sys

version = sys.argv[1].split('.')
suffix = sys.argv[2]

REPO = f'netdata/netdata{suffix}'
GHCR = f'ghcr.io/{REPO}'
QUAY = f'quay.io/{REPO}'

tags = []

for repo in [REPO, GHCR, QUAY]:
    tags.append(':'.join([repo, version[0]]))
    tags.append(':'.join([repo, '.'.join(version[0:2])]))
    tags.append(':'.join([repo, '.'.join(version[0:3])]))

print(','.join(tags))

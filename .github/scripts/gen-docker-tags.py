#!/usr/bin/env python3

import sys

version = sys.argv[1].split('.')
suffix = sys.argv[2]

REPO = f'netdata/netdata{suffix}'

MAJOR = ':'.join([REPO, version[0]])
MINOR = ':'.join([REPO, '.'.join(version[0:2])])
PATCH = ':'.join([REPO, '.'.join(version[0:3])])

print(','.join([MAJOR, MINOR, PATCH]))

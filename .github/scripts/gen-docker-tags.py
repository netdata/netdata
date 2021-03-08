#!/usr/bin/env python3

import sys

REPO = 'netdata/netdata'

version = sys.argv[1].split('.')

MAJOR = ':'.join([REPO, version[0]])
MINOR = ':'.join([REPO, '.'.join(version[0:2])])
PATCH = ':'.join([REPO, '.'.join(version[0:3])])

print(','.join([MAJOR, MINOR, PATCH]))

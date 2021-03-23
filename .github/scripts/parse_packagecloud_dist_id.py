#!/usr/bin/env python3
'''
Parse the PackageCloud distributions JSON data to get a dist ID for uploads.

This takes the JSON distributions data from Packagecloud on stdin and
the package format, distribution name and version as arguments, and
prints either an error message or the parsed distribution ID based on
the arguments.
'''

import json
import sys

fmt = sys.argv[1]      # The package format ('deb' or 'rpm')
distro = sys.argv[2]   # The distro name
version = sys.argv[3]  # The distro version
print(fmt)
print(distro)
print(version)

data = json.load(sys.stdin)
versions = []

for entry in data[fmt]:
    if entry['display_name'] == distro:
        versions = entry['versions']
        break

if not versions:
    print('Could not find version information for the requested distribution.')
    sys.exit(-1)

for entry in versions:
    if entry['version_number'] == version:
        print(entry['id'])
        sys.exit(0)

print('Unable to find id for requested version.')
sys.exit(-1)

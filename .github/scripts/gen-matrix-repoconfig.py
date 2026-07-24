#!/usr/bin/env python3

import json
import sys

from pathlib import Path

SELF = Path(__file__)

sys.path.insert(0, str(SELF.parent.parent.parent / 'packaging' / 'data'))

import distros

data = distros.load_distro_data()
entries = list()

for item in data.include:
    if item.packages is not None:
        entries.append({
            'distro': item.distro,
            'version': item.version,
            'pkgclouddistro': item.packages.repo_distro,
            'format': str(item.packages.type),
            'base_image': item.base_image,
            'platform': data.platform_map[distros.Arch.AMD64],
            'arches': ' '.join(['"' + str(x) + '"' for x in item.packages.arches])
        })

entries.sort(key=lambda k: (k['distro'], k['version']))
matrix = json.dumps({'include': entries}, sort_keys=True)
print(matrix)

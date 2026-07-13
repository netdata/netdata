#!/usr/bin/env python3
'''Generate the build matrix for the EOL check jobs.'''

import json
import sys

from pathlib import Path

SELF = Path(__file__)

sys.path.insert(0, str(SELF.parent.parent.parent / 'packaging' / 'data'))

import distros

data = distros.load_distro_data()
entries = list()

for item in data.include:
    if item.eol_check:
        if isinstance(item.eol_check, str):
            distro = item.eol_check
        else:
            distro = item.distro

        entries.append({
            'distro': distro,
            'release': item.version,
            'full_name': f'{item.distro} {item.version}',
            'lts': 1 if item.eol_lts else 0,
        })

entries.sort(key=lambda k: (k['distro'], k['release']))
matrix = json.dumps({'include': entries}, sort_keys=True)
print(matrix)

#!/usr/bin/env python3

import json
import sys

from pathlib import Path

SELF = Path(__file__)

sys.path.insert(0, str(SELF.parent.parent.parent / 'packaging' / 'data'))

import distros

data = distros.load_distro_data()
entries = []

for item in data.include:
    if item.test.skip_local_build:
        continue

    entry = {
        'artifact_key': item.distro + item.version.replace('.', ''),
        'version': item.version,
    }

    if item.base_image is not None:
        entry['distro'] = item.base_image
    else:
        entry['distro'] = ':'.join([item.distro, item.version])

    if item.env_prep is not None:
        entry['env_prep'] = item.env_prep

    if item.jsonc_removal is not None:
        entry['jsonc_removal'] = item.jsonc_removal

    entries.append(entry)

entries.sort(key=lambda k: k['distro'])
matrix = json.dumps({'include': entries}, sort_keys=True)
print(matrix)

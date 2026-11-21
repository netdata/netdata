#!/usr/bin/env python3

import json
import sys

from ruamel.yaml import YAML

yaml = YAML(typ='safe')
entries = list()
native_only = sys.argv[1] == '1'
include_dem = sys.argv[2] == 'true'
variant_matrix = []

with open('.github/data/distros.yml') as f:
    data = yaml.load(f)

arches = sorted(data['docker_arches'], key=data['arch_order'].index)
for variant in ('regular', 'dem') if include_dem else ('regular',):
    variant_arches = []
    for arch in arches:
        arch_data = data['arch_data'][arch]
        if native_only and arch_data['qemu']:
            continue
        if variant == 'dem' and arch not in ('amd64', 'arm64'):
            continue
        variant_arches.append(arch)
        entries.append({
            'arch': arch,
            'platform': data['platform_map'][arch],
            'runner': arch_data['runner'],
            'qemu': arch_data['qemu'],
            'variant': variant,
        })
    variant_matrix.append({'variant': variant, 'arches': ' '.join(variant_arches)})

dem_arches = {entry['arch'] for entry in entries if entry['variant'] == 'dem'}
test_entries = []
for entry in entries:
    if entry['variant'] == 'regular':
        test_entry = {key: value for key, value in entry.items() if key != 'variant'}
        test_entry['dem'] = entry['arch'] in dem_arches
        test_entries.append(test_entry)

print(json.dumps({
    'build': {'include': entries},
    'publish': {'include': variant_matrix},
    'test': {'include': test_entries},
}, sort_keys=True))

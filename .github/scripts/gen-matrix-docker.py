#!/usr/bin/env python3

import json

from ruamel.yaml import YAML

yaml = YAML(typ='safe')
entries = list()

with open('.github/data/distros.yml') as f:
    data = yaml.load(f)

for arch in data['docker_arches']:
    entries.append({
        'arch': arch,
        'platform': data['platform_map'][arch],
        'runner': data['arch_data'][arch]['runner'],
        'qemu': data['arch_data'][arch]['qemu'],
    })

entries.sort(key=lambda k: data['arch_order'].index(k['arch']))
matrix = json.dumps({'include': entries}, sort_keys=True)
print(matrix)

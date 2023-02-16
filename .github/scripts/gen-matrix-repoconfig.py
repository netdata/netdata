#!/usr/bin/env python3

import json

from ruamel.yaml import YAML

yaml = YAML(typ='safe')
entries = list()

with open('.github/data/distros.yml') as f:
    data = yaml.load(f)

for i, v in enumerate(data['include']):
    if 'packages' in data['include'][i]:
        entries.append({
            'distro': data['include'][i]['distro'],
            'version': data['include'][i]['version'],
            'pkgclouddistro': data['include'][i]['packages']['repo_distro'],
            'format': data['include'][i]['packages']['type'],
            'base_image': data['include'][i]['base_image'] if 'base_image' in data['include'][i] else ':'.join([data['include'][i]['distro'], data['include'][i]['version']]),
            'platform': data['platform_map']['amd64'],
            'arches': ' '.join(['"' + x + '"' for x in data['include'][i]['packages']['arches']])
        })

entries.sort(key=lambda k: (k['distro'], k['version']))
matrix = json.dumps({'include': entries}, sort_keys=True)
print(matrix)

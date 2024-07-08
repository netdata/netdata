#!/usr/bin/env python3
'''Generate the build matrix for the EOL check jobs.'''

import json

from ruamel.yaml import YAML

yaml = YAML(typ='safe')
entries = list()

with open('.github/data/distros.yml') as f:
    data = yaml.load(f)

for item in data['include']:
    if 'eol_check' in item and item['eol_check']:
        if isinstance(item['eol_check'], str):
            distro = item['eol_check']
        else:
            distro = item['distro']

        entries.append({
            'distro': distro,
            'release': item['version'],
            'full_name': f'{ item["distro"] } { item["version"] }',
            'lts': 1 if 'eol_lts' in item and item['eol_lts'] else 0,
        })

entries.sort(key=lambda k: (k['distro'], k['release']))
matrix = json.dumps({'include': entries}, sort_keys=True)
print(matrix)

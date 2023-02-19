#!/usr/bin/env python3

import json

from ruamel.yaml import YAML

yaml = YAML(typ='safe')
entries = []

with open('.github/data/distros.yml') as f:
    data = yaml.load(f)

for i, v in enumerate(data['include']):
    e = {
      'artifact_key': v['distro'] + str(v['version']).replace('.', ''),
      'version': v['version'],
    }

    if 'base_image' in v:
        e['distro'] = ':'.join([v['base_image'], str(v['version'])])
    else:
        e['distro'] = ':'.join([v['distro'], str(v['version'])])

    if 'env_prep' in v:
        e['env_prep'] = v['env_prep']

    if 'jsonc_removal' in v:
        e['jsonc_removal'] = v['jsonc_removal']

    entries.append(e)

entries.sort(key=lambda k: k['distro'])
matrix = json.dumps({'include': entries}, sort_keys=True)
print(matrix)

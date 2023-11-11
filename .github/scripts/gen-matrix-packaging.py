#!/usr/bin/env python3

import json
import sys

from ruamel.yaml import YAML

ALWAYS_RUN_ARCHES = ["amd64", "x86_64"]
SHORT_RUN = sys.argv[1]
yaml = YAML(typ='safe')
entries = []
with open('.github/data/distros.yml') as f:
    data = yaml.load(f)

run_limited = bool(int(SHORT_RUN))
for i, v in enumerate(data['include']):
    if 'packages' in data['include'][i]:
        entries.extend(
            {
                'distro': data['include'][i]['distro'],
                'version': data['include'][i]['version'],
                'repo_distro': data['include'][i]['packages']['repo_distro'],
                'format': data['include'][i]['packages']['type'],
                'base_image': data['include'][i]['base_image']
                if 'base_image' in data['include'][i]
                else ':'.join(
                    [
                        data['include'][i]['distro'],
                        data['include'][i]['version'],
                    ]
                ),
                'platform': data['platform_map'][arch],
                'arch': arch,
            }
            for arch in data['include'][i]['packages']['arches']
            if arch in ALWAYS_RUN_ARCHES or not run_limited
        )
entries.sort(key=lambda k: (data['arch_order'].index(k['arch']), k['distro'], k['version']))
matrix = json.dumps({'include': entries}, sort_keys=True)
print(matrix)

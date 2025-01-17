#!/usr/bin/env python3

import json
import sys

from ruamel.yaml import YAML

ALWAYS_RUN_ARCHES = ["amd64", "x86_64"]
SHORT_RUN = sys.argv[1]
yaml = YAML(typ='safe')
entries = list()
run_limited = False

with open('.github/data/distros.yml') as f:
    data = yaml.load(f)

if bool(int(SHORT_RUN)):
    run_limited = True

for i, v in enumerate(data['include']):
    if 'packages' in data['include'][i]:
        for arch in data['include'][i]['packages']['arches']:
            if arch in ALWAYS_RUN_ARCHES or not run_limited:
                entries.append({
                    'distro': data['include'][i]['distro'],
                    'version': data['include'][i]['version'],
                    'repo_distro': data['include'][i]['packages']['repo_distro'],
                    'format': data['include'][i]['packages']['type'],
                    'base_image': data['include'][i]['base_image'] if 'base_image' in data['include'][i] else ':'.join([data['include'][i]['distro'], data['include'][i]['version']]),
                    'builder_rev': data['include'][i]['packages']['builder_rev'],
                    'platform': data['platform_map'][arch],
                    'bundle_sentry': data['include'][i]['bundle_sentry'][arch],
                    'arch': arch,
                    'runner': data['arch_data'][arch]['runner'],
                    'qemu': data['arch_data'][arch]['qemu'],
                })

entries.sort(key=lambda k: (data['arch_order'].index(k['arch']), k['distro'], k['version']))
matrix = json.dumps({'include': entries}, sort_keys=True)
print(matrix)

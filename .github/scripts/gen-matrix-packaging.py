#!/usr/bin/env python3

import json
import sys

from pathlib import Path

SELF = Path(__file__)

sys.path.insert(0, str(SELF.parent.parent.parent / 'packaging' / 'data'))

import distros

data = distros.load_distro_data()
ALWAYS_RUN_ARCHES = (
    distros.Arch.AMD64,
    distros.Arch.X86_64,
    distros.Arch.I386,
    distros.Arch.ARMHF,
    distros.Arch.AARCH64,
    distros.Arch.ARM64,
)
SHORT_RUN = sys.argv[1]
entries = list()
run_limited = False

if bool(int(SHORT_RUN)):
    run_limited = True

for item in data.include:
    if item.packages is not None:
        for arch in item.packages.arches:
            if arch in ALWAYS_RUN_ARCHES or not run_limited:
                entry = {
                    'distro': item.distro,
                    'version': item.version,
                    'repo_distro': item.packages.repo_distro,
                    'format': str(item.packages.type),
                    'builder_rev': item.packages.builder_rev,
                    'platform': str(data.platform_map[arch]),
                    'bundle_sentry': item.bundle_sentry[arch],
                    'arch': str(arch),
                    'runner': data.arch_data[arch].runner,
                    'qemu': data.arch_data[arch].qemu,
                }

            if item.base_image is not None:
                entry['distro'] = item.base_image
            else:
                entry['distro'] = ':'.join([item.distro, item.version])

            entries.append(entry)

entries.sort(key=lambda k: (data.arch_order.index(distros.Arch(k['arch'])), k['distro'], k['version']))
matrix = json.dumps({'include': entries}, sort_keys=True)
print(matrix)

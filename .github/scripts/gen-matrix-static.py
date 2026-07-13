#!/usr/bin/env python3

import json
import sys

from pathlib import Path

SELF = Path(__file__)

sys.path.insert(0, str(SELF.parent.parent.parent / 'packaging' / 'data'))

import distros

data = distros.load_distro_data()
entries = list()
native_only = sys.argv[1]

for arch in data.static_arches:
    if native_only == '1' and data.arch_data[arch].qemu:
        continue

    entries.append({
        'arch': str(arch),
        'runner': data.arch_data[arch].runner,
        'qemu': data.arch_data[arch].qemu,
    })

entries.sort(key=lambda k: data.arch_order.index(distros.Arch(k['arch'])))
matrix = json.dumps({'include': entries}, sort_keys=True)
print(matrix)

#!/usr/bin/env python3

import json
import os
import pathlib

from packaging.version import parse

SCRIPT_PATH = pathlib.Path(__file__).parents[0]
REPO_ROOT = SCRIPT_PATH.parents[1]
GO_SRC = REPO_ROOT / 'src' / 'go'

GITHUB_OUTPUT = pathlib.Path(os.environ['GITHUB_OUTPUT'])

version = parse('1.0.0')
modules = []

for modfile in GO_SRC.glob('**/go.mod'):
    moddata = modfile.read_text()

    for line in moddata.splitlines():
        if line.startswith('go '):
            version = max(version, parse(line.split()[1]))
            break

    for main in modfile.parent.glob('**/main.go'):
        mainpath = main.relative_to(modfile.parent).parent

        if 'examples' in mainpath.parts:
            continue

        # Skip ibmdplugin as it requires CGO
        if 'ibmdplugin' in mainpath.parts:
            continue

        modules.append({
            'module': str(modfile.parent),
            'version': str(version),
            'build_target': f'github.com/netdata/netdata/go/plugins/{str(mainpath)}/',
        })

with GITHUB_OUTPUT.open('a') as f:
    f.write(f'matrix={json.dumps(modules)}\n')

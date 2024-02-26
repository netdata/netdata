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
    modules.append(str(modfile.parent))
    moddata = modfile.read_text()

    for line in moddata.splitlines():
        if line.startswith('go '):
            version = max(version, parse(line.split()[1]))
            break

with GITHUB_OUTPUT.open('a') as f:
    f.write(f'version={ str(version) }\n')

with GITHUB_OUTPUT.open('a') as f:
    f.write(f'matrix={ json.dumps({"module": modules}) }\n')

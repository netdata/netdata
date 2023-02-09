#!/bin/env python3

#%%

import os
rootdir = './health/health.d'

#%%


for subdir, dirs, files in os.walk(rootdir):
    for file in files:
        if file.endswith('.conf'):
            with open(os.path.join(subdir, file), 'r') as file:
                data = file.read()
                blocks = [block.split('\n') for block in data.split('\n\n')]
                blocks = [block for block in blocks if len(block)>=3]
                print(blocks)
                break

#%%
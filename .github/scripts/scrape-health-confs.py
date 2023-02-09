#!/bin/env python3

#%%

import os
rootdir = './health/health.d'

#%%

configs = []
for subdir, dirs, files in os.walk(rootdir):
    for file in files:
        if file.endswith('.conf'):
            with open(os.path.join(subdir, file), 'r') as file:
                data = file.read()
                blocks = [block.split('\n') for block in data.split('\n\n')]
                blocks = [block for block in blocks if len(block)>=3]
                for block in blocks:
                    conf_dict = {'file': file}
                    for line in block:
                        if ':' in line and not line.startswith('#'):
                            print(line)    
                            line_parts = line.strip().split(':')
                            key = line_parts[0].strip()
                            value = line_parts[1].strip()
                            conf_dict[key] = value
                    configs.append(conf_dict)

print(configs)

#%%
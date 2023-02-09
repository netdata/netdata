#!/bin/env python3

#%%

import os
import re
import pandas as pd

#root_dir = './health/health.d'
#out_dir = './docs/'
root_dir = '../../health/health.d'
out_dir = '../../docs/'


#%%

keyword_list = [
    'template','on','class','type','component','lookup',
    'units','every','crit','delay','info','to','warn','os',
    'hosts','calc','families','options','alarm','module','green',
    'red','host labels','plugin'
    ]

configs = []
for sub_dir, dirs, files in os.walk(root_dir):
    for file in files:
        if file.endswith('.conf'):
            with open(os.path.join(sub_dir, file), 'r') as file:
                data = file.read()
                data = re.sub(r'\\\s*\n', ' ', data)
                blocks = [block.split('\n') for block in data.split('\n\n')]
                blocks = [block for block in blocks if len(block)>=3]
                for block in blocks:
                    conf_dict = {'file': file.name}
                    re.sub(r'\\\s*\n[^:]', ' ', data)
                    for line in block:
                        if any(f'{word}:' in line for word in keyword_list) and not line.startswith('#'):
                            line_parts = line.strip().split(':', 1)
                            key = line_parts[0].strip()
                            value = line_parts[1].strip()
                            conf_dict[key] = value
                    configs.append(conf_dict)

#%%

df_configs = pd.DataFrame(configs)
print(df_configs)

df_configs.to_csv(f'{out_dir}/health_confs.csv', index=False)

#%%
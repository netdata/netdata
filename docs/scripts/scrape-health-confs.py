#!/bin/env python3
""" 
Scrapes health.d/*.conf files for health checks and writes to csv.
"""


#%%

import os
import re
import pandas as pd

root_dir = './health/health.d'
out_dir = './docs/'
#root_dir = '../../health/health.d'
#out_dir = '../../docs/'

#%%

keyword_list = [
    'template','on','class','type','component','lookup',
    'units','every','crit','delay','info','to','warn','os',
    'hosts','calc','families','options','alarm','module','green',
    'red','host labels','plugin'
    ]

# a list of config dictionaries
configs = []

# walk through the health.d directory
for sub_dir, dirs, files in os.walk(root_dir):
    # for each file in the directory
    for file in files:
        # if the file is a .conf file
        if file.endswith('.conf'):
            # open the file
            with open(os.path.join(sub_dir, file), 'r') as file:
                # read the file
                data = file.read()
                # join lines ending with a backslash, typical in info lines
                data = re.sub(r'\\\s*\n', ' ', data)
                # split the file into blocks based on newlines as is typical in health.d/*.conf files
                blocks = [block.split('\n') for block in data.split('\n\n')]
                # remove blocks with less than 3 lines, irrelevant
                blocks = [block for block in blocks if len(block)>=3]
                # for each block
                for block in blocks:
                    # create a dictionary for the block and add the file name we are dealing with
                    conf_dict = {'file': file.name.split('/')[-1]}
                    # for each line in the block
                    for line in block:
                        # if the line contains a keyword and is not a comment
                        if any(f'{word}:' in line for word in keyword_list) and not line.startswith('#'):
                            # split the line into key and value
                            line_parts = line.strip().split(':', 1)
                            key = line_parts[0].strip()
                            value = line_parts[1].strip()
                            # add key:values to dictionary
                            conf_dict[key] = value
                    # if the dictionary has more than one key, it is a valid config
                    if len(conf_dict.keys()) > 1:
                        # add the dictionary to the list of configs
                        configs.append(conf_dict)

#%%

# create a dataframe from the list of configs
df_configs = pd.DataFrame(configs)

print(df_configs)

# write the dataframe to csv
df_configs.to_csv(f'{out_dir}/health_confs.csv', index=False)

#%%
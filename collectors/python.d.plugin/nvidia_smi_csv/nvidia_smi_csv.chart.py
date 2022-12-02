#!/usr/bin/env python

# -*- coding: utf-8 -*-
# Description: postfix netdata python.d module
# Author: Pawel Krupa (paulfantom)
# SPDX-License-Identifier: GPL-3.0-or-later

from bases.FrameworkServices.ExecutableService import ExecutableService
import re
import functools
import copy

NVIDIA_SMI_COMMAND = 'nvidia-smi --query-gpu=name,temperature.gpu,power.draw,utilization.gpu,utilization.memory --format=csv'

# ORDER values are dynamically added
# They will look similar to this but with indexes attached
ORDER = [
#    'temperature.gpu',
#    'utilization.gpu',
#    'utilization.memory',
#    'power.draw', 
]

CHARTS_TEMPLATE = {
    'temperature.gpu__INDEX__': {
        'options': [None, 'Temperature', 'C', '__PRODUCT__', 'nvidia-smi.temperature', 'line'],
        'lines': [
            ['temperature.gpu__INDEX__', None, 'absolute'], #Something that looks like this is added below
        ]
    },
    'utilization.gpu__INDEX__': {
        'options': [None, 'GPU Utilization', '%', '__PRODUCT__', 'nvidia-smi.gpu_utilization', 'line'],
        'lines': [
            ['utilization.gpu__INDEX__', None, 'absolute'],
        ]
    },
    'utilization.memory__INDEX__': {
        'options': [None, 'Memory Utilization', '%', '__PRODUCT__', 'nvidia-smi.memory_utilization', 'line'],
        'lines': [
            ['utilization.memory__INDEX__', None, 'absolute'],
        ]
    },
    'power.draw__INDEX__': {
        'options': [None, 'Power Draw', 'W', '__PRODUCT__', 'nvidia-smi.power.draw', 'line'],
        'lines': [
            ['power.draw__INDEX__', None, 'absolute'],
        ]
    },
}

CHARTS = {}

def clean_data(raw):
    headers = raw[0].split(", ")
    for index, header in enumerate(headers):
        headers[index] = re.match(r"(\S+) *", header)[1]

    fields = raw[1:]
    for index, value in enumerate(fields):
        fields[index] = value.split(", ")
        for sub_index, value in enumerate(fields[index]):
            if "name" in headers[sub_index]:
                continue
            fields[index][sub_index] =  float(re.match(r"(\S+) *", value)[1])

    return headers, fields
def stats_to_dict(headers, fields):
    data_dict = dict()
    for i, values in enumerate(fields):
        # Iterates over both headers and fields at the same time
        # It makes code a bit more readable and understandable, I think
        for value, header in zip(values, headers):
            header_val = header+str(i)
            data_dict[header_val] = value

    return data_dict

def replace_chars(string, old, new):
    return string.replace(old, new)

# "Replace a string in list of lists": https://stackoverflow.com/a/13782720
def recursively_apply(l, f):
    for n, i in enumerate(l):
        if type(i) is list:
            l[n] = recursively_apply(l[n], f)
        elif type(i) is str:
            l[n] = f(i)
    return l

def create_charts(data_dict):
    current_product = ""
    # It was either I set this to a negative value or subtract one from it every time I use this value
    # I do not like this solution of just starting it at -1, but it works.
    gpu_index = -1
    for key_name in data_dict.keys():
        if "name" in key_name:
            current_product = data_dict[key_name]
            # Incrementing it by one to Overwrite the index to parts later.
            gpu_index += 1
            continue
        ORDER.append(key_name)
        # Grabbing an independent copy that does not link to the main TEMPLATE to modify 
        template_copy = copy.deepcopy(CHARTS_TEMPLATE)
        for template_key, template_value in CHARTS_TEMPLATE.items():
            if template_key.replace('__INDEX__', str(gpu_index)) == key_name:
                CHARTS[key_name] = template_copy.pop(template_key)
                break

        replace_product = functools.partial(replace_chars, old='__PRODUCT__', new=current_product)
        replace_index = functools.partial(replace_chars, old='__INDEX__', new=str(gpu_index))
        # Searching the dictionary values recursively for strings to change to corrected value
        for key, value in CHARTS[key_name].items():
            recursively_apply(value, replace_product)
            recursively_apply(value, replace_index)
        

class Service(ExecutableService):
    def __init__(self, configuration=None, name=None):
        ExecutableService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.command = NVIDIA_SMI_COMMAND

    def _get_data(self):
        """
        Format data received from shell command
        :return: dict
        """
        try:
            data_dict = dict()
            raw = self._get_raw_data()
            headers, fields = clean_data(raw)
            data_dict = stats_to_dict(headers, fields)
            if len(CHARTS) == 0:
                create_charts(data_dict)
            return(data_dict)
        except (ValueError, AttributeError):
            return None

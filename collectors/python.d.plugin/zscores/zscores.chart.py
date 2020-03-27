# -*- coding: utf-8 -*-
# Description: zscores netdata python.d module
# Author: andrewm4894
# SPDX-License-Identifier: GPL-3.0-or-later

from random import SystemRandom, randint

from bases.FrameworkServices.SimpleService import SimpleService

import requests
import numpy as np
import pandas as pd


priority = 2

ORDER = [
    'zscores',
]

CHARTS = {
    'zscores': {
        'options': [None, 'Z Scores', 'value', 'zscores', 'anomalies.zscores', 'line'],
        'lines': [
            ['system.cpu.user', None, 1, 1000],
            ['system.cpu.system', None, 1, 1000],
            ['system.cpu.softirq', None, 1, 1000],
            ['system.load.load1', None, 1, 1000],
            ['system.load.load5', None, 1, 1000],
            ['system.load.load15', None, 1, 1000],
            ['system.io.in', None, 1, 1000],
            ['system.io.out', None, 1, 1000],
            ['system.pgpgio.in', None, 1, 1000],
            ['system.pgpgio.out', None, 1, 1000],
            ['system.net.sent', None, 1, 1000],
            ['system.net.received', None, 1, 1000],
            ['system.processes.running', None, 1, 1000],
            ['system.intr.interrupts', None, 1, 1000],
        ]
    },
}

HOST_PORT = '127.0.0.1:19999'
N = 500

def get_raw_data(self, host=None):

    if host is None:
        host = HOST_PORT

    data = dict()

    for chart in ['system.cpu', 'system.load', 'system.io', 'system.pgpgio', 'system.net', 'system.processes', 'system.intr']:

        # get data
        url = f'http://{host}/api/v1/data?chart={chart}&after=-{N}&format=json'
        response = requests.get(url)
        response_json = response.json()
        raw_data = response_json['data']
        df = pd.DataFrame(raw_data, columns=response_json['labels'])
        df = df.set_index('time').dropna().sort_index()

        # get rolling df
        r = df.expanding()
        # get mean
        m = r.mean().shift(1)
        # get standard deviation
        s = r.std(ddof=0).shift(1)
        # get zscore
        df = (df - m) / s

        df = df.dropna(axis=1, how='all').tail(1)

        for col in df.columns:
            if col not in self.charts['zscores']:
                try:
                    self.charts['zscores'].add_dimension([f'{chart}.{col}'])
                except:
                    pass
            data[f'{chart}.{col}'] = abs(df[col].values[0]) * 1000

    return data


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.random = SystemRandom()

    @staticmethod
    def check():
        return True

    def get_data(self):

        data = get_raw_data(self)

        return data


#%%

#data = get_raw_data('london.my-netdata.io')
#print(data)

#%%

#%%

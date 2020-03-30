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
        'options': [None, 'Z Scores', 'value', 'zscores', 'zscores.zscores', 'line'],
        'lines': [
        ]
    },
}


HOST_PORT = '127.0.0.1:19999'
N = 500
CHARTS_IN_SCOPE = [
    'system.cpu', 'system.load', 'system.ram', 'system.io', 'system.pgpgio', 'system.net', 'system.ip', 'system.ipv6',
    'system.processes', 'system.intr', 'system.forks', 'system.softnet_stat'
]


def get_raw_data(self=None, host=None):

    if host is None:
        host = HOST_PORT

    data = dict()

    for chart in CHARTS_IN_SCOPE:

        # get data
        url = f'http://{host}/api/v1/data?chart={chart}&after=-{N}&format=json'
        response = requests.get(url)
        response_json = response.json()
        raw_data = response_json['data']

        # create df
        df = pd.DataFrame(raw_data, columns=response_json['labels'])
        df = df.set_index('time').dropna().sort_index()

        print(df.shape)
        print(df.head())

        # get mean
        m = df.mean()
        # get standard deviation
        s = df.std(ddof=0)
        # get zscore
        df = (df - m) / s
        df = df.tail(1).dropna(axis=1, how='all').clip(-10, 10)

        for col in df.columns:

            dimension_id = f'{chart}.{col}'

            if self:

                if dimension_id not in self.charts['zscores']:

                    self.charts['zscores'].add_dimension([dimension_id, None, 'absolute', 1, 100])

            data[dimension_id] = df[col].values[0] * 1000

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

#data = get_raw_data(host='london.my-netdata.io')
#print(data)

#%%

#%%

# -*- coding: utf-8 -*-
# Description: example netdata python.d module
# Author: Put your name here (your github login)
# SPDX-License-Identifier: GPL-3.0-or-later

from random import SystemRandom

import requests
import pandas as pd
from bases.FrameworkServices.SimpleService import SimpleService
from bases.utils import get_allmetrics

priority = 1

HOST_PORT = '127.0.0.1:19999'
CHARTS_IN_SCOPE = ['system.cpu', 'system.load']
N = 5

ORDER = [
    'system.cpu',
    'system.load',
    'random'
]

CHARTS = {
    'system.cpu': {
        'options': [None, 'system.cpu', f'system.cpu (ma{N})', 'cpu', 'system_cpu', 'line'],
        'lines': [
        ]
    },
    'system.load': {
        'options': [None, 'system.load', f'system.load (ma{N})', 'load', 'system_load', 'line'],
        'lines': [
        ]
    },
}


def data_to_df(data, mode='wide'):
    df = pd.DataFrame([item for sublist in data for item in sublist], columns=['time', 'chart', 'variable', 'value'])
    if mode == 'wide':
        df = df.drop_duplicates().pivot(index='time', columns='variable', values='value').ffill()
    return df


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.random = SystemRandom()
        self.data = []

    @staticmethod
    def check():
        return True

    def append_data(self, data):
        self.data.append(data)

    def get_data(self):

        self.append_data(get_allmetrics(host=HOST_PORT, charts=CHARTS_IN_SCOPE))
        self.data = self.data[-N:]
        self.debug(f"self.data={self.data}")
        df = data_to_df(self.data)
        df = df.mean().to_frame().transpose()
        self.debug(df.shape)
        self.debug(df.head())

        data = dict()

        for col in df.columns:
            self.debug(f"col={col}")
            parts = col.split('.')
            chart = '.'.join(parts[0:2])
            name = parts[-1]
            self.debug(f"chart={chart}")
            if name not in self.charts[chart]:
                self.charts[chart].add_dimension([name, name, 'absolute', 1, 1000])
            data[name] = df[col].values[0] * 1000

        return data

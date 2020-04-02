# -*- coding: utf-8 -*-
# Description: example netdata python.d module
# Author: andrewm4894
# SPDX-License-Identifier: GPL-3.0-or-later

from random import SystemRandom

import requests
import pandas as pd
from bases.FrameworkServices.SimpleService import SimpleService

priority = 1

HOST_PORT = '127.0.0.1:19999'
CHARTS_IN_SCOPE = [
    'system.cpu', 'system.load', 'system.io', 'system.pgpgio', 'system.net', 'system.ip', 'system.ipv6', 'system.intr'
]
CHART_TYPES = {'stacked': ['system.cpu']}
N = 5

ORDER = CHARTS_IN_SCOPE

CHARTS = dict()
for CHART_IN_SCOPE in CHARTS_IN_SCOPE:
    if CHART_IN_SCOPE in CHART_TYPES['stacked']:
        chart_type = 'stacked'
    else:
        chart_type = 'line'
    # set chart options
    name = CHART_IN_SCOPE
    title = CHART_IN_SCOPE
    units = f'{CHART_IN_SCOPE} (ma{N})'
    family = CHART_IN_SCOPE.split('.')[-1]
    context = CHART_IN_SCOPE.replace('.', '_')
    CHARTS[CHART_IN_SCOPE] = {
        'options': [name, title, units, family, context, chart_type],
        'lines': []
    }


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

    def get_allmetrics(self, host: str = '127.0.0.1:19999', charts: list = None) -> list:
        if charts is None:
            charts = ['system.cpu']
        url = f'http://{host}/api/v1/allmetrics?format=json'
        response = requests.get(url)
        raw_data = response.json()
        data = []
        for k in raw_data:
            if k in charts:
                time = raw_data[k]['last_updated']
                dimensions = raw_data[k]['dimensions']
                for dimension in dimensions:
                    data.append([time, k, f"{k}.{dimensions[dimension]['name']}", dimensions[dimension]['value']])
        return data

    def data_to_df(self, data, mode='wide'):
        df = pd.DataFrame([item for sublist in data for item in sublist],
                          columns=['time', 'chart', 'variable', 'value'])
        if mode == 'wide':
            df = df.drop_duplicates().pivot(index='time', columns='variable', values='value').ffill()
        return df

    def append_data(self, data):
        self.data.append(data)

    def get_data(self):

        self.append_data(self.get_allmetrics(host=HOST_PORT, charts=CHARTS_IN_SCOPE))
        self.data = self.data[-N:]
        df = self.data_to_df(self.data)
        df = df.mean().to_frame().transpose()

        data = dict()

        for col in df.columns:
            parts = col.split('.')
            chart = '.'.join(parts[0:2])
            name = parts[-1]
            if name not in self.charts[chart]:
                self.charts[chart].add_dimension([name, name, 'absolute', 1, 1000])
            data[name] = df[col].values[0] * 1000

        return data

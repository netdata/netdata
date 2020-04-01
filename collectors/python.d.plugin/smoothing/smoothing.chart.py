# -*- coding: utf-8 -*-
# Description: example netdata python.d module
# Author: Put your name here (your github login)
# SPDX-License-Identifier: GPL-3.0-or-later

from random import SystemRandom

import requests
import pandas as pd
from bases.FrameworkServices.SimpleService import SimpleService

priority = 1

ORDER = [
    'system.cpu',
    'random'
]

CHARTS = {
    'system.cpu': {
        'options': [None, 'system.cpu', 'system.cpu', 'smoothing', 'system_cpu', 'line'],
        'lines': [
        ]
    },
    'random': {
        'options': [None, 'random', 'random', 'smoothing', 'random', 'line'],
        'lines': [
        ]
    }
}

HOST_PORT = '127.0.0.1:19999'
CHARTS_IN_SCOPE = ['system.cpu']
N = 2


def get_allmetrics(host: str = None, charts: list = None) -> list:
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


def data_to_df(data):
    df = pd.DataFrame([item for sublist in data for item in sublist], columns=['time', 'chart', 'variable', 'value'])
    return df

def df_long_to_wide(df):
    df = df.drop_duplicates().pivot(index='time', columns='variable', values='value').ffill()
    return df


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.random = SystemRandom()
        self.counter = 1
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
        print(df.shape)
        print(df.head())
        df = df_long_to_wide(df)
        print(df.shape)
        print(df.head())
        df = df.mean().to_frame().transpose()
        print(df.shape)
        print(df.head())

        data = dict()

        for col in df.columns:
            print(col)
            chart = '.'.join(col.split('.')[0:2])
            print(chart)
        #    if col not in self.charts[chart]:
        #        self.charts[chart].add_dimension([col, col, 'absolute', 1, 1000])
        #    data[col] = df[col].values[0] * 1000

        #print(data)

        for i in range(1, 10):
            dimension_id = ''.join(['random', str(i)])

            if dimension_id not in self.charts['random']:
                self.charts['random'].add_dimension([dimension_id])

            data[dimension_id] = self.random.randint(0, 100)

        return data

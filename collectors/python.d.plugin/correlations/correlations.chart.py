# -*- coding: utf-8 -*-
# Description: correlations netdata python.d module
# Author: andrewm4894
# SPDX-License-Identifier: GPL-3.0-or-later

from random import SystemRandom, randint

from bases.FrameworkServices.SimpleService import SimpleService

import requests
import pandas as pd


priority = 3

ORDER = [
    'correlations',
]

CHARTS = {
    'correlations': {
        'options': [None, 'Cross Correlations', 'value', 'correlations', 'correlations.correlations', 'line'],
        'lines': [
        ]
    },
}


HOST_PORT = '127.0.0.1:19999'
N = 60
CHARTS_IN_SCOPE = [
    'system.cpu', 'system.load', 'system.ram', 'system.io', 'system.pgpgio', 'system.net', 'system.ip', 'system.ipv6',
    'system.processes', 'system.intr', 'system.forks', 'system.softnet_stat'
]
CHARTS_IN_SCOPE = ['system.cpu', 'system.load']


def get_allmetrics(host: str = None, charts: list = None):
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


def get_raw_data(host: str = None, after: int = 500, charts: list = None) -> pd.DataFrame:

    df = pd.DataFrame(columns=['time'])

    # get all relevant data
    for chart in charts:

        # get data
        url = f'http://{host}/api/v1/data?chart={chart}&after=-{after}&format=json'
        response = requests.get(url)
        response_json = response.json()
        raw_data = response_json['data']

        # create df
        df_chart = pd.DataFrame(raw_data, columns=response_json['labels'])
        df_chart = df_chart.rename(
            columns={col: f'{chart}.{col}' for col in df_chart.columns[df_chart.columns != 'time']}
        )
        df = df.merge(df_chart, on='time', how='outer')

    df = df.set_index('time')
    df = df.ffill()

    return df


def process_data(self=None, df: pd.DataFrame = None) -> dict:

    data = dict()
    print(df.shape)
    print(df.head(10))
    print('... corr ...')
    df = df.corr()
    print(df.shape)
    print(df.head(10))
    print('... stack ...')
    df = df.stack()
    print(df.shape)
    print(df.head(10))
    print('... to_frame ...')
    df = df.to_frame()
    print(df.shape)
    print(df.head(10))
    print('... droplevel ...')
    df.columns = df.columns.droplevel(0)
    print(df.shape)
    print(df.head(10))
    print('... enumerate ...')
    df.columns = [f'{x}_{i}' for i, x in enumerate(df.columns, 1)]
    print(df.shape)
    print(df.head(10))
    print('... reset_index ...')
    df = df.reset_index()
    print(df.shape)
    print(df.head(10))
    print('... rename ...')
    df = df.rename(columns={0: 'value'})
    df['variable'] = df['level_0'] + '__' + df['level_1']
    df = df[df['level_0']!=df['level_1']]
    df = df[['variable', 'value']]
    df['idx'] = 1
    df = df.pivot(index='idx', columns='variable', values='value')

    for col in df.columns:
        if self:
            self.counter += 1
            if col not in self.charts['correlations']:
                self.charts['correlations'].add_dimension([col, None, 'absolute', 1, 1000])
        data[col] = df[col].values[0] * 1000

    return data


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
        print(f"length of self.data is {len(self.data)}")
        print(self.data)
        df = data_to_df(self.data)
        print(df.shape)
        print(df.head(10))
        df = df_long_to_wide(df)
        print(df.shape)
        print(df.head(10))
        data = process_data(self, df)

        return data


#%%

#import time

#data1 = get_allmetrics(host='london.my-netdata.io', charts=CHARTS_IN_SCOPE)
#time.sleep(1)
#data2 = get_allmetrics(host='london.my-netdata.io', charts=CHARTS_IN_SCOPE)
#time.sleep(1)
#data3 = get_allmetrics(host='london.my-netdata.io', charts=CHARTS_IN_SCOPE)

#%%

#data = list()
#data.append(data1)
#data.append(data2)
#data.append(data3)
#df = data_to_df(data)

#%%

#df = df_long_to_wide(df)

#%%

#df_tmp.corr()

#%%

import pandas as pd

df = pd.DataFrame([(.2, .3), (.0, .6), (.6, .0), (.2, .1)],
                  columns=['dogs', 'dogs'])
df_corr = df.corr().stack().to_frame()

#%%

df_corr = df_corr.reset_index()

#%%
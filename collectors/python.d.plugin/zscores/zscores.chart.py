# -*- coding: utf-8 -*-
# Description: zscores netdata python.d module
# Author: andrewm4894
# SPDX-License-Identifier: GPL-3.0-or-later

from random import SystemRandom, randint

from bases.FrameworkServices.SimpleService import SimpleService

import requests
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
AFTER = 50
CHARTS_IN_SCOPE = [
    'system.cpu', 'system.load', 'system.ram', 'system.io', 'system.pgpgio', 'system.net', 'system.ip', 'system.ipv6',
    'system.processes', 'system.intr', 'system.forks', 'system.softnet_stat'
]


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

    m = df.mean()
    s = df.std(ddof=0)
    # get zscore
    df = (df - m) / s
    df = df.tail(1).dropna(axis=1, how='all').clip(-10, 10)

    for col in df.columns:
        if self:
            if col not in self.charts['zscores']:
                self.charts['zscores'].add_dimension([col, None, 'absolute', 1, 1000])
        data[col] = df[col].values[0] * 1000

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

        df = get_raw_data(host=HOST_PORT, after=AFTER, charts=CHARTS_IN_SCOPE)
        data = process_data(self, df)

        return data


#%%

#df = get_raw_data(host='london.my-netdata.io', after=AFTER, charts=CHARTS_IN_SCOPE)
#data = process_data(df=df)
#print(df)
#print(data)

#%%

#%%
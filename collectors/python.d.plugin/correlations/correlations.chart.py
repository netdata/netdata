# -*- coding: utf-8 -*-
# Description: zscores netdata python.d module
# Author: andrewm4894
# SPDX-License-Identifier: GPL-3.0-or-later

from random import SystemRandom

import requests
import numpy as np
import pandas as pd
from bases.FrameworkServices.SimpleService import SimpleService

priority = 2

HOST_PORT = '127.0.0.1:19999'
CHARTS_IN_SCOPE = [
    'system.cpu', 'system.load', 'system.io', 'system.pgpgio',
    'system.net', 'system.ip', 'system.ipv6', 'system.intr'
]
N = 100
CORRELATION_THOLD = 0.8

ORDER = [
    'metric_correlations'
]

CHARTS = {
    'metric_correlations': {
        'options': [None, 'Metric Correlations', '(var1,var2)', 'correlations', 'correlations.correlations', 'stacked'],
        'lines': []
    },
}


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.random = SystemRandom()
        self.data = []
        self.mean = dict()
        self.sigma = dict()


    @staticmethod
    def check():
        return True

    @staticmethod
    def get_allmetrics(host: str = '127.0.0.1:19999', charts: list = None) -> list:
        """
        Hits the allmetrics endpoint on `host` filters for `charts` of interest and saves data into a list
        :param host: host to pull data from <str>
        :param charts: charts to filter to <list>
        :return: list of lists where each element is a metric from allmetrics <list>
        """
        if charts is None:
            charts = ['system.cpu']
        url = f'http://{host}/api/v1/allmetrics?format=json'
        raw_data = requests.get(url).json()
        data = []
        for k in raw_data:
            if k in charts:
                time = raw_data[k]['last_updated']
                dimensions = raw_data[k]['dimensions']
                for dimension in dimensions:
                    # [time, chart, name, value]
                    data.append([time, k, f"{k}.{dimensions[dimension]['name']}", dimensions[dimension]['value']])
        return data

    @staticmethod
    def data_to_df(data, mode='wide'):
        """
        Parses data list of list's from allmetrics and formats it as a pandas dataframe.
        :param data: list of lists where each element is a metric from allmetrics <list>
        :param mode: used to determine if we want pandas df to be long (row per metric) or wide (col per metric) format <str>
        :return: pandas dataframe of the data <pd.DataFrame>
        """
        df = pd.DataFrame([item for sublist in data for item in sublist],
                          columns=['time', 'chart', 'variable', 'value'])
        if mode == 'wide':
            df = df.drop_duplicates().pivot(index='time', columns='variable', values='value').ffill()
        return df

    def append_data(self, data):
        self.data.append(data)

    def get_data(self):

        # empty dict to collect data points into
        data = dict()

        # get latest data from allmetrics
        latest_observations = self.get_allmetrics(host=HOST_PORT, charts=CHARTS_IN_SCOPE)
        self.append_data(latest_observations)

        # limit size of data maintained to last n
        self.data = self.data[-N:]


        # pull data into a pandas df
        df = self.data_to_df(self.data)

        df = df.corr()
        df = df.rename_axis("var1", axis="index")
        df = df.rename_axis("var2", axis="columns")
        df = df.stack().to_frame().reset_index().rename(columns={0: 'value'})
        df['variable'] = df['var1'] + '__' + df['var2']
        df = df[df['var1'] != df['var2']]
        df = df[['variable', 'value']]
        df['idx'] = 1
        df = df.pivot(index='idx', columns='variable', values='value')
        self.debug('df')
        self.debug(df.shape)
        self.debug(df.head())

        # add to chart data
        for col in df.columns:
            dimension_id = col.replace('system.', '').replace('__', ' , ')
            dimension_id = f'({dimension_id})'
            value = df[col].values[0]
            if abs(value) >= CORRELATION_THOLD:
                if dimension_id not in self.charts['metric_correlations']:
                    self.charts['metric_correlations'].add_dimension([dimension_id, dimension_id, 'absolute', 1, 100])
                data[dimension_id] = value * 100

        return data















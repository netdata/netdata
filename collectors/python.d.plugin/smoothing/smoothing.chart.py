# -*- coding: utf-8 -*-
# Description: smoothing netdata python.d module
# Author: andrewm4894
# SPDX-License-Identifier: GPL-3.0-or-later


import requests
import pandas as pd
from bases.FrameworkServices.SimpleService import SimpleService

priority = 1

HOST_PORT = '127.0.0.1:19999'
CHARTS_IN_SCOPE = [
    'system.cpu', 'system.load', 'system.io', 'system.pgpgio',
    'system.net', 'system.ip', 'system.ipv6', 'system.intr'
]
CHART_TYPES = {'stacked': ['system.cpu']}
N = 5

ORDER = CHARTS_IN_SCOPE

# define charts based on whats in scope
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
        self.data = []

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

        # get data from allmetrics and append to self
        self.append_data(self.get_allmetrics(host=HOST_PORT, charts=CHARTS_IN_SCOPE))

        # limit size of data maintained to last n
        self.data = self.data[-N:]

        # pull data into a pandas df
        df = self.data_to_df(self.data)

        # do calculations
        df = df.mean().to_frame().transpose()

        # save results to data
        for col in df.columns:
            parts = col.split('.')
            chart, name = ('.'.join(parts[0:2]), parts[-1])
            if name not in self.charts[chart]:
                self.charts[chart].add_dimension([name, name, 'absolute', 1, 1000])
            data[name] = df[col].values[0] * 1000

        return data

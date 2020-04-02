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
CHART_TYPES = {'stacked': ['system.cpu']}
N = 50
RECALC_EVERY = 5

ORDER = [
    'zscores',
]

CHARTS = {
    'zscores': {
        'options': [None, 'Z Scores', 'value', 'zscores', 'zscores.zscores', 'line'],
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

        # get data from allmetrics and append to self
        latest_observations = self.get_allmetrics(host=HOST_PORT, charts=CHARTS_IN_SCOPE)
        df_latest = self.data_to_df([latest_observations]).mean().to_dict()
        self.debug('df_latest')
        self.debug(df_latest)

        # limit size of data maintained to last n
        self.data = self.data[-N:]

        if self.runs_counter % RECALC_EVERY == 0:
            # pull data into a pandas df
            df_data = self.data_to_df(self.data)
            # do calculations
            self.mean = df_data.mean().to_dict()
            self.sigma = df_data.std().to_dict()

        for metric in df_latest.keys():
            self.debug(metric)
            x = df_latest.get(metric,0)
            mu = self.mean.get(metric,0)
            sigma = self.sigma.get(metric,0)
            self.debug(f'x={x}')
            self.debug(f'mu={mu}')
            self.debug(f'sigma={sigma}')
            if sigma == 0:
                z = 0
            else:
                z = float((x - mu) / sigma)
            z = np.clip(z, -10, 10)
            self.debug(f'z={z}')
            if metric not in self.charts['zscores']:
                self.charts['zscores'].add_dimension([metric, metric, 'absolute', 1, 1000])
            data[metric] = z * 1000


        ## save results to data
        #for col in df.columns:
        #    parts = col.split('.')
        #    chart, name = ('.'.join(parts[0:2]), parts[-1])
        #    if name not in self.charts[chart]:
        #        self.charts[chart].add_dimension([name, name, 'absolute', 1, 1000])
        #    data[name] = df[col].values[0] * 1000

        #for i in range(1, 4):
        #    dimension_id = ''.join(['random', str(i)])
        #    if dimension_id not in self.charts['zscores']:
        #        self.charts['zscores'].add_dimension([dimension_id])
        #    data[dimension_id] = self.random.randint(0, 100)

        # append data
        self.append_data(latest_observations)

        return data



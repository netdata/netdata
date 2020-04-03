# -*- coding: utf-8 -*-
# Description: anomalies netdata python.d module
# Author: andrewm4894
# SPDX-License-Identifier: GPL-3.0-or-later

from random import SystemRandom

import requests
import numpy as np
import pandas as pd
from bases.FrameworkServices.SimpleService import SimpleService

priority = 3

HOST_PORT = '127.0.0.1:19999'
CHARTS_IN_SCOPE = [
    'system.cpu'
]
N = 500
RECALC_EVERY = 50
ZSCORE_CLIP = 10

ORDER = [
    'anomaly_score',
    'anomaly_flag'
]

CHARTS = {
    'anomaly_score': {
        'options': [None, 'Anomaly Scores', 'name.chart', 'anomaly_scores', 'anomalies.scores', 'line'],
        'lines': []
    },
    'anomaly_flag': {
        'options': [None, 'Anomaly Flag', 'name.chart', 'anomaly_flag', 'anomalies.anomalies', 'stacked'],
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
        data_latest = self.data_to_df([latest_observations]).mean().to_dict()

        # limit size of data maintained to last n
        self.data = self.data[-N:]

        # recalc if needed
        if self.runs_counter % RECALC_EVERY == 0:
            # pull data into a pandas df
            df_data = self.data_to_df(self.data)
            # update mean and sigma
            self.mean = df_data.mean().to_dict()
            self.sigma = df_data.std().to_dict()

        # process each metric and add to data
        for metric in data_latest.keys():
            metric_rev = '.'.join(reversed(metric.split('.')))
            metric_rev_flag = f'{metric_rev}_flag'
            x = data_latest.get(metric, 0)
            mu = self.mean.get(metric, 0)
            sigma = self.sigma.get(metric, 0)
            self.debug(f'metric={metric}, x={x}, mu={mu}, sigma={sigma}')
            # calculate z score
            if sigma == 0:
                z = 0
            else:
                z = (x - mu) / sigma
            # clip z score
            z = np.clip(z, -ZSCORE_CLIP, ZSCORE_CLIP)
            self.debug(f'z={z}')
            if metric_rev not in self.charts['anomaly_score']:
                self.charts['anomaly_score'].add_dimension([metric_rev, metric_rev, 'absolute', 1, 100])
            if metric_rev_flag not in self.charts['anomaly_flag']:
                self.charts['anomaly_flag'].add_dimension([metric_rev_flag, metric_rev_flag, 'absolute', 1, 1])
            data[metric_rev] = z * 100
            data[metric_rev_flag] = 1 if abs(z) > 3 else 0

        # append latest data
        self.append_data(latest_observations)

        return data



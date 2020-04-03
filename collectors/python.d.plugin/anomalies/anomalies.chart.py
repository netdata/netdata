# -*- coding: utf-8 -*-
# Description: anomalies netdata python.d module
# Author: andrewm4894
# SPDX-License-Identifier: GPL-3.0-or-later

from random import SystemRandom

import requests
import numpy as np
import pandas as pd
from pyod.models.hbos import HBOS
from bases.FrameworkServices.SimpleService import SimpleService

priority = 3

HOST_PORT = '127.0.0.1:19999'
CHARTS_IN_SCOPE = [
    'system.cpu'
]
N = 100
REFIT_EVERY = 50
CONTAMINATION = 0.01

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

        # define model
        model = HBOS(contamination=CONTAMINATION)

        # get latest data from allmetrics
        latest_observations = self.get_allmetrics(host=HOST_PORT, charts=CHARTS_IN_SCOPE)
        data_latest = self.data_to_df([latest_observations]).mean().to_dict()

        # limit size of data maintained to last n
        self.data = self.data[-N:]

        # recalc if needed
        if self.runs_counter % REFIT_EVERY == 0:
            # pull data into a pandas df
            df_data = self.data_to_df(self.data)
            # refit the model
            model.fit(df_data.values)

        # get anomaly score and flag
        if hasattr(model, "decision_scores_"):
            anomaly_flag = model.predict(data_latest.values)[-1]
            anomaly_score = model.decision_function(data_latest.values)[-1]
        else:
            anomaly_flag = 0
            anomaly_score = 0

        if 'cpu_score' not in self.charts['anomaly_score']:
            self.charts['anomaly_score'].add_dimension(['cpu_score', 'cpu_score', 'absolute', 1, 100])
        if 'cpu_flag' not in self.charts['anomaly_flag']:
            self.charts['anomaly_flag'].add_dimension(['cpu_flag', 'cpu_flag', 'absolute', 1, 100])
        data['cpu_score'] = anomaly_score
        data['cpu_flag'] = anomaly_flag

        # append latest data
        self.append_data(latest_observations)

        return data



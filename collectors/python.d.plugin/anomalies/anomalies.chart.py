# -*- coding: utf-8 -*-
# Description: anomalies netdata python.d module
# Author: andrewm4894
# SPDX-License-Identifier: GPL-3.0-or-later


import requests
import pandas as pd
from pyod.models.knn import KNN
from pyod.models.hbos import HBOS
from pyod.models.cblof import CBLOF
#from pyod.models.auto_encoder import AutoEncoder
from bases.FrameworkServices.SimpleService import SimpleService

priority = 3

HOST = '127.0.0.1:19999'
CHARTS_IN_SCOPE = [
    'system.cpu', 'system.load', 'system.io', 'system.pgpgio', 'system.ram', 'system.net', 'system.ip', 'system.ipv6',
    'system.processes', 'system.ctxt', 'system.idlejitter', 'system.intr', 'system.softirqs', 'system.softnet_stat'
]
TRAIN_MAX_N = 60*10
FIT_EVERY = 60
MODEL_CONFIG = {
    'type': 'cblof',
    'kwargs': {'contamination': 0.001},
    'score': True,
    'prob': True,
    'flag': True,
}

ORDER = [
    'score',
    'probability',
    'anomaly_flag'
]

CHARTS = {
    'score': {
        'options': [None, 'Anomaly Score', 'Anomaly Score', 'score', 'anomalies.score', 'line'],
        'lines': []
    },
    'probability': {
        'options': [None, 'Anomaly Probability', 'Anomaly Probability', 'probability', 'anomalies.probability', 'line'],
        'lines': []
    },
    'flag': {
        'options': [None, 'Anomaly Flag', 'Anomaly Flag', 'flag', 'anomalies.flag', 'stacked'],
        'lines': []
    },
}


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.data = []
        self.models = dict()
        self.charts_in_scope = CHARTS_IN_SCOPE
        self.model_config = MODEL_CONFIG
        self.fit_every = FIT_EVERY
        self.train_max_n = TRAIN_MAX_N
        self.host = HOST


    @staticmethod
    def check():
        return True

    @staticmethod
    def data_to_df(data: list, mode: str = 'wide', charts: list = None) -> pd.DataFrame:
        """
        Parses data list of list's from allmetrics and formats it as a pandas dataframe.
        :param data: list of lists where each element is a metric from allmetrics <list>
        :param mode: used to determine if we want pandas df to be long (row per metric) or wide (col per metric) format <str>
        :param charts: filter data just for charts of interest <list>
        :return: pandas dataframe of the data <pd.DataFrame>
        """
        if charts:
            data = [item for sublist in data for item in sublist if item[1] in charts]
        else:
            data = [item for sublist in data for item in sublist]
        df = pd.DataFrame(data, columns=['time', 'chart', 'variable', 'value'])
        if mode == 'wide':
            df = df.drop_duplicates().pivot(index='time', columns='variable', values='value').ffill()
        return df

    def get_allmetrics(self) -> list:
        """
        Hits the allmetrics endpoint on `host` filters for `charts` of interest and saves data into a list
        :param host: host to pull data from <str>
        :param charts: charts to filter to <list>
        :return: list of lists where each element is a metric from allmetrics <list>
        """
        if self.charts_in_scope is None:
            self.charts_in_scope = ['system.cpu']
        url = 'http://{}/api/v1/allmetrics?format=json'.format(self.host)
        raw_data = requests.get(url).json()
        data = []
        for k in raw_data:
            if k in self.charts_in_scope:
                time = raw_data[k]['last_updated']
                dimensions = raw_data[k]['dimensions']
                for dimension in dimensions:
                    # [time, chart, name, value]
                    data.append(
                        [time, k, "{}.{}".format(k, dimensions[dimension]['name']), dimensions[dimension]['value']]
                    )
        return data

    def append_data(self, data):
        self.data.append(data)

    def get_data(self):

        # empty dict to collect data points into
        data = dict()

        # get latest data from allmetrics
        latest_observations = self.get_allmetrics()

        for chart in self.charts_in_scope:

            self.debug("chart={}".format(chart))

            data_latest = self.data_to_df([latest_observations], charts=[chart]).mean().to_frame().transpose()
            self.debug('data_latest={}'.format(data_latest))

            if chart not in self.models:
                if self.model_config['type'] == 'hbos':
                    self.models[chart] = HBOS(**self.model_config['kwargs'])
                elif self.model_config['type'] == 'knn':
                    self.models[chart] = KNN(**self.model_config['kwargs'])
                elif self.model_config['type'] == 'cblof':
                    self.models[chart] = CBLOF(**self.model_config['kwargs'])
                #elif self.model_config['type'] == 'auto_encoder':
                #    self.models[chart] = AutoEncoder(**self.model_config['kwargs'])

            chart_score = "{}_score".format(chart.replace('system.', ''))
            chart_prob = "{}_prob".format(chart.replace('system.', ''))
            chart_flag = "{}_flag".format(chart.replace('system.', ''))

            # refit if needed
            if self.runs_counter % self.fit_every == 0:
                # pull data into a pandas df
                df_data = self.data_to_df(self.data, charts=[chart])
                # refit the model
                self.models[chart].fit(df_data.values)

            # get anomaly score and flag
            if hasattr(self.models[chart], "decision_scores_"):
                X = data_latest.values
                anomaly_flag = self.models[chart].predict(X)[-1]
                anomaly_score = self.models[chart].decision_function(X)[-1]
                self.debug('X={}'.format(X))
                self.debug('anomaly_score={}'.format(anomaly_score))
                self.debug('anomaly_flag={}'.format(anomaly_flag))

                if self.model_config['prob']:
                    anomaly_prob = self.models[chart].predict_proba(X)[-1][1]
                    self.debug('anomaly_prob={}'.format(anomaly_prob))

            else:
                anomaly_flag = 0
                anomaly_score = 0
                if self.model_config['prob']:
                    anomaly_prob = 0

            if chart_score not in self.charts['score']:
                self.charts['score'].add_dimension([chart_score, chart_score, 'absolute', 1, 100])
            if chart_flag not in self.charts['flag']:
                self.charts['flag'].add_dimension([chart_flag, chart_flag, 'absolute', 1, 1])
            data[chart_score] = anomaly_score * 100
            data[chart_flag] = anomaly_flag

            if self.model_config['prob']:
                if chart_prob not in self.charts['probability']:
                    self.charts['probability'].add_dimension([chart_prob, chart_prob, 'absolute', 1, 100])
                data[chart_prob] = anomaly_prob * 100

        # append latest data
        self.append_data(latest_observations)
        # limit size of data maintained to last n
        self.data = self.data[-self.train_max_n:]

        return data



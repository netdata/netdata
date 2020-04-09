# -*- coding: utf-8 -*-
# Description: anomalies netdata python.d module
# Author: andrewm4894
# SPDX-License-Identifier: GPL-3.0-or-later


import requests
import pandas as pd
#from pyod.models.knn import KNN
from pyod.models.hbos import HBOS
#from pyod.models.cblof import CBLOF
from bases.FrameworkServices.SimpleService import SimpleService

priority = 3
update_every = 1

HOST = '127.0.0.1:19999'
CHARTS_IN_SCOPE = [
    'system.cpu', 'system.load', 'system.io', 'system.pgpgio', 'system.ram', 'system.net', 'system.ip', 'system.ipv6',
    'system.processes', 'system.ctxt', 'system.idlejitter', 'system.intr', 'system.softirqs', 'system.softnet_stat'
]
TRAIN_MAX_N = 60*60
TRAIN_SAMPLE_PCT = 1
FIT_EVERY = 60*5
LAGS_N = 2
SMOOTHING_N = 3
MODEL_CONFIG = {
    'kwargs': {'contamination': 0.001},
    'score': False,
    'prob': True,
    'flag': True,
}

ORDER = [
    'score',
    'prob',
    'flag'
]

CHARTS = {
    'score': {
        'options': [None, 'Anomaly Score', 'Anomaly Score', 'score', 'anomalies.score', 'line'],
        'lines': []
    },
    'prob': {
        'options': [None, 'Anomaly Probability', 'Anomaly Probability', 'prob', 'anomalies.prob', 'line'],
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
        self.models = {chart: HBOS(**MODEL_CONFIG['kwargs']) for chart in CHARTS_IN_SCOPE}
        self.charts_in_scope = CHARTS_IN_SCOPE
        self.model_config = MODEL_CONFIG
        self.fit_every = FIT_EVERY
        self.train_max_n = TRAIN_MAX_N
        self.host = HOST
        self.lags_n = LAGS_N
        self.smoothing_n = SMOOTHING_N
        self.prediction = {chart: {} for chart in CHARTS_IN_SCOPE}
        self.train_sample_pct = TRAIN_SAMPLE_PCT

    @staticmethod
    def check():
        return True

    @staticmethod
    def data_to_df(data: list, mode: str = 'wide', charts: list = None, n: int = None) -> pd.DataFrame:
        """
        Parses data list of list's from allmetrics and formats it as a pandas dataframe.
        :param data: list of lists where each element is a metric from allmetrics <list>
        :param mode: used to determine if we want pandas df to be long (row per metric) or wide (col per metric) format <str>
        :param charts: filter data just for charts of interest <list>
        :param n: filter data just for n last rows <list>
        :return: pandas dataframe of the data <pd.DataFrame>
        """
        if n:
            data = data[-n:]
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
        # limit size of data maintained
        self.data = self.data[-self.train_max_n:]

    def make_x(self, df):
        # do smoothing
        if self.smoothing_n >= 2:
            df = df.rolling(self.smoothing_n).mean().dropna()
        # add lags
        if self.lags_n >= 1:
            df = pd.concat([df.shift(n) for n in range(self.lags_n + 1)], axis=1).dropna()
        # sample if specified
        if 0 < self.train_sample_pct < 1:
            df = df.sample(frac=self.train_sample_pct)
        return df.values

    def model_fit(self, chart):
        # get train data
        df = self.data_to_df(self.data, charts=[chart])
        X_train = self.make_x(df)
        self.debug('X_train.shape={}'.format(X_train.shape))
        # fit model
        self.models[chart].fit(X_train)

    def can_predict(self, chart):
        return True if hasattr(self.models[chart], "decision_scores_") else False

    def model_predict(self, chart):
        prediction = dict()
        X_predict = self.make_x(self.data_to_df(self.data, charts=[chart], n=(1+((self.lags_n + self.smoothing_n)*5))))
        if self.model_config['score']:
            prediction['score'] = self.models[chart].decision_function(X_predict)[-1]
        if self.model_config['prob']:
            prediction['prob'] = self.models[chart].predict_proba(X_predict)[-1][1]
        if self.model_config['flag']:
            prediction['flag'] = self.models[chart].predict(X_predict)[-1]
        self.prediction[chart] = prediction

    def update_chart_dim(self, chart, dimension_id, title=None, algorithm='absolute', multiplier=1, divisor=1):
        if dimension_id not in self.charts[chart]:
            self.charts[chart].add_dimension([dimension_id, title, algorithm, multiplier, divisor])
        return True

    def get_data(self):

        # empty dict to collect data points into
        data = dict()

        # get latest data from allmetrics
        latest_observations = self.get_allmetrics()

        # append latest data
        self.append_data(latest_observations)

        # get scores and models for each chart
        for chart in self.charts_in_scope:

            self.debug("chart={}".format(chart))

            #self.model_init(chart)

            # get prediction
            if self.can_predict(chart):
                self.model_predict(chart)

            # refit if needed
            if self.runs_counter % self.fit_every == 0:
                self.model_fit(chart)

            # insert charts and data

            if self.model_config['score']:
                score = "{}_score".format(chart.replace('system.', ''))
                self.update_chart_dim('score', score, divisor=100)
                data[score] = self.prediction[chart].get('score', 0) * 100

            if self.model_config['prob']:
                prob = "{}_prob".format(chart.replace('system.', ''))
                self.update_chart_dim('prob', prob, divisor=100)
                data[prob] = self.prediction[chart].get('prob', 0) * 100

            if self.model_config['flag']:
                flag = "{}_flag".format(chart.replace('system.', ''))
                self.update_chart_dim('flag', flag)
                data[flag] = self.prediction[chart].get('flag', 0)

        return data



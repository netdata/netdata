# -*- coding: utf-8 -*-
# Description: anomalies netdata python.d module
# Author: andrewm4894
# SPDX-License-Identifier: GPL-3.0-or-later

import requests
import numpy as np
import pandas as pd

# basic/traditional models
#from pyod.models.hbos import HBOS
from pyod.models.cblof import CBLOF
#from pyod.models.pca import PCA
#from pyod.models.loda import LODA
#from pyod.models.abod import ABOD
# more fancy models
#from pyod.models.iforest import IForest
#from pyod.models.vae import VAE
#from pyod.models.auto_encoder import AutoEncoder

from bases.FrameworkServices.SimpleService import SimpleService

priority = 3
update_every = 2

HOST = '127.0.0.1:19999'

# charts to generate anomaly scores for
CHARTS_IN_SCOPE = [
    'system.cpu', 'system.load', 'system.io', 'system.pgpgio', 'system.ram', 'system.net', 'system.ip', 'system.ipv6',
    'system.processes', 'system.ctxt', 'system.idlejitter', 'system.intr', 'system.softirqs', 'system.softnet_stat',
    'system.cpu_pressure', 'system.io_some_pressure', 'system.active_processes', 'system.entropy'
]

# model configuration
MODEL_CONFIG = {
    'models': {chart: CBLOF(**{'contamination': 0.001}) for chart in CHARTS_IN_SCOPE},
    'do_score': False,
    'do_prob': True,
    'do_flag': True,
    'diffs_n': 1,
    'lags_n': 2,
    'smoothing_n': 2,
    'data_max_n': 60*60,
    'train_max_n': 60*60,
    'train_min_n': 60,
    'train_sample_pct': 1,
    'fit_every_n': 60*5,
    'flags_min_n': 2,
    'flags_window_n': 3
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
        self.models = MODEL_CONFIG['models']
        self.charts_in_scope = CHARTS_IN_SCOPE
        self.host = HOST
        self.predictions = {chart: [[0, 0, 0]] for chart in CHARTS_IN_SCOPE}
        self.do_score = MODEL_CONFIG.get('do_score', False)
        self.do_prob = MODEL_CONFIG.get('do_prob', True)
        self.do_flag = MODEL_CONFIG.get('do_flag', True)
        self.diffs_n = MODEL_CONFIG.get('diffs_n', 0)
        self.lags_n = MODEL_CONFIG.get('lags_n', 0)
        self.smoothing_n = MODEL_CONFIG.get('smoothing_n', 0)
        self.data_max_n = MODEL_CONFIG.get('data_max_n', 60*10)
        self.train_max_n = MODEL_CONFIG.get('train_max_n', 60 * 10)
        self.train_min_n = MODEL_CONFIG.get('train_min_n', 60)
        self.train_sample_pct = MODEL_CONFIG.get('train_sample_pct', 1)
        self.fit_every_n = MODEL_CONFIG.get('fit_every_n', 60*5)
        self.predictions_keep_n = MODEL_CONFIG.get('predictions_keep_n', 5)
        self.flags_min_n = MODEL_CONFIG.get('flags_min_n', 1)
        self.flags_window_n = MODEL_CONFIG.get('flags_window_n', 2)

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
        df = pd.DataFrame(
            data, columns=['time', 'chart', 'variable', 'value']
        ).groupby(['time', 'chart', 'variable']).mean().reset_index()
        if mode == 'wide':
            df = df.drop_duplicates().pivot(index='time', columns='variable', values='value').ffill()
        return df

    def get_allmetrics(self) -> list:
        """
        Hits the allmetrics endpoint on and saves data into a list
        :return: list of lists where each element is a metric from allmetrics e.g. [[time, chart, name, value]] <list>
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
        # append data to self
        self.data.append(data)
        # limit size of data maintained
        self.data = self.data[-self.data_max_n:]

    def make_x(self, df: pd.DataFrame):
        """
        Given the input dataframe apply model relevant pre-processing to
        get X feature vector for the model to train or score on.
        :param df: host to pull data from <pd.DataFrame>
        :return: Numpy array that represents X feature vector to feed into the model.
        """
        # take diffs
        if self.diffs_n >= 1:
            df = df.diff(self.diffs_n).dropna()
        # do smoothing
        if self.smoothing_n >= 2:
            df = df.rolling(self.smoothing_n).mean().dropna()
        # add lags
        if self.lags_n >= 1:
            df = pd.concat([df.shift(n) for n in range(self.lags_n + 1)], axis=1).dropna()
        # sample if specified
        if self.train_max_n < self.data_max_n:
            df = df.sample(n=self.train_max_n)
        X = df.values
        return X

    def model_fit(self, chart):
        """
        Fit (or refit) the model for a specified chart, using the available data.
        :param chart: the chart for which to fit the model on.
        """
        # get train data
        df = self.data_to_df(self.data, charts=[chart])
        X_train = self.make_x(df)
        self.debug('X_train.shape={}'.format(X_train.shape))
        # fit model
        self.models[chart].fit(X_train)

    def can_predict(self, chart):
        """
        Check if the model has been fitted and so is able to predict.
        :param chart: the chart for which to check for.
        """
        return True if hasattr(self.models[chart], "decision_scores_") else False

    def model_predict(self, chart):
        """
        Generate a prediction for a model and store it.
        :param chart: the chart for which to generate a prediction for.
        """
        prediction = []
        # create feature vector on recent data to make predictions on
        X_predict = self.make_x(self.data_to_df(self.data, charts=[chart], n=(1+((self.lags_n + self.smoothing_n)*5))))
        self.debug('X_predict.shape={}'.format(X_predict.shape))
        #self.debug('X_predict={}'.format(X_predict))
        # make score, prob, flag as specified and keep most recent as current prediction
        if self.do_score:
            prediction.append(self.models[chart].decision_function(X_predict)[-1])
        else:
            prediction.append(0)
        if self.do_prob:
            prediction.append(self.models[chart].predict_proba(X_predict)[-1][1])
        else:
            prediction.append(0)
        if self.do_flag:
            prediction.append(self.models[chart].predict(X_predict)[-1])
        else:
            prediction.append(0)
        # update prediction for the chart
        self.predictions[chart].append(prediction)
        # keep most recent n
        self.predictions[chart] = self.predictions[chart][-self.predictions_keep_n:]
        self.debug('len(self.predictions[{}])={}'.format(chart, len(self.predictions[chart])))
        self.debug('self.predictions[{}]={}'.format(chart, self.predictions[chart]))

    def update_chart_dim(self, chart, dimension_id, title=None, algorithm='absolute', multiplier=1, divisor=1):
        """
        Check if the chart has a specific dimension and if not then add it.
        :param chart: the chart for which to check for.
        :param dimension_id:
        :param title:
        :param algorithm:
        :param multiplier:
        :param divisor:
        """
        if dimension_id not in self.charts[chart]:
            self.charts[chart].add_dimension([dimension_id, title, algorithm, multiplier, divisor])
        return True

    def get_data(self):

        # empty dict to collect data points into
        data = dict()

        # get and append latest data
        self.append_data(self.get_allmetrics())

        # get scores and models for each chart
        for chart in self.charts_in_scope:

            self.debug("chart={}".format(chart))

            # get prediction
            if self.can_predict(chart):
                self.debug("self.can_predict({})={}".format(chart, self.can_predict(chart)))
                self.model_predict(chart)

            # refit if needed
            if (self.runs_counter % self.fit_every_n == 0) or (self.runs_counter == self.train_min_n):
                self.debug("fitting model")
                self.model_fit(chart)

            # insert charts and data

            if self.do_score:
                score_label = "{}_score".format(chart.replace('system.', ''))
                self.update_chart_dim('score', score_label, divisor=100)
                score_value = self.predictions[chart][-1][0]
                self.debug("{}={}".format(score_label, score_value))
                data[score_label] = score_value * 100

            if self.do_prob:
                prob_label = "{}_prob".format(chart.replace('system.', ''))
                self.update_chart_dim('prob', prob_label, divisor=100)
                prob_value = self.predictions[chart][-1][1]
                self.debug("{}={}".format(prob_label, prob_value))
                data[prob_label] = prob_value * 100

            if self.do_flag:
                flag_label = "{}_flag".format(chart.replace('system.', ''))
                self.update_chart_dim('flag', flag_label)
                # work out if to flag based on recent flags history
                if self.flags_min_n > 1:
                    # check if self.flags_min_n or more flags in last self.flags_window_n flags from predictions
                    flag_count = np.sum([prediction[2] for prediction in self.predictions[chart][-self.flags_window_n:]])
                    if flag_count >= self.flags_min_n:
                        flag_value = 1
                    else:
                        flag_value = 0
                else:
                    # just get most recent value
                    flag_value = self.predictions[chart][-1][2]
                self.debug("{}={}".format(flag_label, flag_value))
                data[flag_label] = flag_value

        # add averages

        if self.do_score:
            score_label = 'mean_score'
            self.update_chart_dim('score', score_label, divisor=100)
            data[score_label] = np.mean([data[k] for k in data if 'score' in k])

        if self.do_prob:
            prob_label = 'mean_prob'
            self.update_chart_dim('prob', prob_label, divisor=100)
            data[prob_label] = np.mean([data[k] for k in data if 'prob' in k])

        return data



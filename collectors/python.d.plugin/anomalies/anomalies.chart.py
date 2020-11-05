# -*- coding: utf-8 -*-
# Description: anomalies netdata python.d module
# Author: andrewm4894
# SPDX-License-Identifier: GPL-3.0-or-later

import time
from datetime import datetime
import re

import requests
import numpy as np
import pandas as pd
from netdata_pandas.data import get_data, get_allmetrics
from pyod.models.hbos import HBOS
from pyod.models.pca import PCA
from pyod.models.loda import LODA
from pyod.models.iforest import IForest
from pyod.models.cblof import CBLOF
from pyod.models.feature_bagging import FeatureBagging
from pyod.models.copod import COPOD
from sklearn.preprocessing import MinMaxScaler

from bases.FrameworkServices.SimpleService import SimpleService


ORDER = ['probability', 'anomaly']

CHARTS = {
    'probability': {
        'options': ['probability', 'Anomaly Probability', 'probability', 'anomalies', 'anomalies.probability', 'line'],
        'lines': []
    },
    'anomaly': {
        'options': ['anomaly', 'Anomaly', 'count', 'anomalies', 'anomalies.anomaly', 'stacked'],
        'lines': []
    },
}


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.protocol = self.configuration.get('protocol', 'http')
        self.host = self.configuration.get('host', '127.0.0.1:19999')
        self.charts_regex = re.compile(self.configuration.get('charts_regex','system\..*'))
        self.charts_in_scope = list(filter(self.charts_regex.match, [c for c in requests.get(f'{self.protocol}://{self.host}/api/v1/charts').json()['charts'].keys()]))
        self.charts_to_exclude = self.configuration.get('charts_to_exclude', '').split(',')
        if len(self.charts_to_exclude) > 0:
            self.charts_in_scope = [c for c in self.charts_in_scope if c not in self.charts_to_exclude]
        self.model = self.configuration.get('model', 'pca')
        self.train_max_n = self.configuration.get('train_max_n', 100000)
        self.train_n_secs = self.configuration.get('train_n_secs', 14400)
        self.offset_n_secs = self.configuration.get('offset_n_secs', 0)
        self.train_every_n = self.configuration.get('train_every_n', 900)
        self.contamination = self.configuration.get('contamination', 0.001)
        self.lags_n = self.configuration.get('lags_n', 5)
        self.smooth_n = self.configuration.get('smooth_n', 3)
        self.diffs_n = self.configuration.get('diffs_n', 1)
        self.custom_models = self.configuration.get('custom_models', None)
        self.custom_models_normalize = bool(self.configuration.get('custom_models_normalize', False))
        if self.custom_models:
            self.custom_models_names = [m['name'] for m in self.custom_models]
            self.custom_models_dims = [i for s in [m['dimensions'].split(',') for m in self.custom_models] for i in s]
            self.custom_models_charts = list(set([c.split('|')[0] for c in self.custom_models_dims]))
            self.custom_models_dims_renamed = []
            for m in self.custom_models:
                self.custom_models_dims_renamed.extend([f"{m['name']}.{d}" for d in m['dimensions'].split(',')])
            self.models_in_scope = list(set(self.charts_in_scope + self.custom_models_names))
            self.charts_in_scope = list(set(self.charts_in_scope + self.custom_models_charts))
        else:
            self.models_in_scope = self.charts_in_scope
        if self.model == 'pca':
            self.models = {model: PCA(contamination=self.contamination) for model in self.models_in_scope}
        elif self.model == 'loda':
            self.models = {model: LODA(contamination=self.contamination) for model in self.models_in_scope}
        elif self.model == 'iforest':
            self.models = {model: IForest(n_estimators=50, bootstrap=True, behaviour='new', contamination=self.contamination) for model in self.models_in_scope}
        elif self.model == 'cblof':
            self.models = {model: CBLOF(n_clusters=3, contamination=self.contamination) for model in self.models_in_scope}
        elif self.model == 'feature_bagging':
            self.models = {model: FeatureBagging(base_estimator=PCA(contamination=self.contamination), contamination=self.contamination) for model in self.models_in_scope}
        elif self.model == 'copod':
            self.models = {model: COPOD(contamination=self.contamination) for model in self.models_in_scope}
        elif self.model == 'hbos':
            self.models = {model: HBOS(contamination=self.contamination) for model in self.models_in_scope}
        else:
            self.models = {model: HBOS(contamination=self.contamination) for model in self.models_in_scope}
        self.scaler = MinMaxScaler()
        self.fitted_at = {}
        self.df_predict = pd.DataFrame()
        self.df_allmetrics = pd.DataFrame()
        self.expected_cols = []
        self.data_latest = {}

    @staticmethod
    def check():
        return True

    def validate_charts(self, name, data, algorithm='absolute', multiplier=1, divisor=1):
        """If dimension not in chart then add it.
        """
        for dim in data:
            if dim not in self.charts[name]:
                self.charts[name].add_dimension([dim, dim, algorithm, multiplier, divisor])

    @staticmethod
    def get_array_cols(colnames, arr, starts_with):
        """Given an array and list of colnames, return subset of cols from array where colname startswith starts_with.

        :param colnames <list>: list of colnames corresponding to arr.
        :param arr <np.ndarray>: numpy array we want to select a subset of cols from.
        :param starts_with <str>: the string we want to return all columns that start with given str value.
        :return: <np.ndarray> subseted array.
        """
        cols_idx = [i for i, x in enumerate(colnames) if x.startswith(starts_with)]

        return arr[:, cols_idx]

    def add_custom_models_dims(self, df):
        """Given a df, select columns used by custom models, add custom model name as prefix, and append to df.

        :param df <pd.DataFrame>: dataframe to append new renamed columns to.
        :return: <pd.DataFrame> dataframe with additional columns added relating to the specified custom models.
        """
        df_custom = df[self.custom_models_dims].copy()
        df_custom.columns = self.custom_models_dims_renamed
        df = df.join(df_custom)

        return df

    def set_expected_cols(self, df):
        """Given a df, set expected columns that determine expected schema for both training and prediction.

        :param df <pd.DataFrame>: dataframe to use to determine expected cols from.
        """
        self.expected_cols = sorted(list(set(df.columns)))
        # if using custom models then may need to remove some unused cols as data comes in per chart
        if self.custom_models:
            ignore_cols = []
            for col in self.expected_cols:
                for chart in self.custom_models_charts:
                    if col.startswith(chart):
                        if col not in self.custom_models_dims:
                            ignore_cols.append(col)
            self.expected_cols = [c for c in self.expected_cols if c not in ignore_cols]

    def make_features(self, arr, colnames, train=False):
        """Take in numpy array and preprocess accordingly by taking diffs, smoothing and adding lags.

        :param arr <np.ndarray>: numpy array we want to make features from.
        :param colnames <list>: list of colnames corresponding to arr.
        :param train <bool>: True if making features for training, in which case need to fit_transform scaler and maybe sample train_max_n.
        :return: (<np.ndarray>, <list>) tuple of list of colnames of features and transformed numpy array.
        """

        def lag(arr, n):
            res = np.empty_like(arr)
            res[:n] = np.nan
            res[n:] = arr[:-n]

            return res

        arr = np.nan_to_num(arr)

        if self.custom_models_normalize:
            # normalize just custom model columns which will be the last len(self.custom_models_dims) cols in arr.
            if train:
                arr_custom = self.scaler.fit_transform(arr[:,-len(self.custom_models_dims):])
            else:
                arr_custom = self.scaler.transform(arr[:,-len(self.custom_models_dims):])
            arr = arr[:,:-len(self.custom_models_dims)]
            arr = np.concatenate((arr, arr_custom), axis=1)

        if self.diffs_n > 0:
            arr = np.diff(arr, self.diffs_n, axis=0)
            arr = arr[~np.isnan(arr).any(axis=1)]

        if self.smooth_n > 1:
            arr = np.cumsum(arr, axis=0, dtype=float)
            arr[self.smooth_n:] = arr[self.smooth_n:] - arr[:-self.smooth_n]
            arr = arr[self.smooth_n - 1:] / self.smooth_n
            arr = arr[~np.isnan(arr).any(axis=1)]

        if self.lags_n > 0:
            colnames = colnames + [f'{col}_lag{lag}' for lag in range(1, self.lags_n + 1) for col in colnames]
            arr_orig = np.copy(arr)
            for lag_n in range(1, self.lags_n + 1):
                arr = np.concatenate((arr, lag(arr_orig, lag_n)), axis=1)
            arr = arr[~np.isnan(arr).any(axis=1)]

        if train:
            if len(arr) > self.train_max_n:
                arr = arr[np.random.randint(arr.shape[0], size=self.train_max_n), :]

        arr = np.nan_to_num(arr)

        return arr, colnames

    def train(self, models_to_train=None):
        """Pull required training data and train a model for each specified model.

        :return:
        """
        now = datetime.now().timestamp()
        before = int(now) - self.offset_n_secs
        after =  before - self.train_n_secs

        # get training data
        df_train = get_data(self.host, self.charts_in_scope, after=after, before=before, sort_cols=True, numeric_only=True, protocol=self.protocol, float_size='float32').ffill()
        self.set_expected_cols(df_train)
        df_train = df_train[self.expected_cols]
        if self.custom_models:
            df_train = self.add_custom_models_dims(df_train)

        # make feature vector
        data = df_train.values
        X, feature_colnames = self.make_features(data, list(df_train.columns), train=True)

        # train model
        self.try_fit(X, feature_colnames)
        self.info(f'training complete in {round(time.time() - now, 2)} seconds (runs_counter={self.runs_counter}, model={self.model}, train_n_secs={self.train_n_secs}, models={len(self.fitted_at)}, n_fit_success={self.n_fit_success}, n_fit_fails={self.n_fit_fail}).')
        self.debug(f'self.fitted_at = {self.fitted_at}')

    def try_fit(self, X, feature_colnames, models_to_train=None):
        """Try fit each model and try to fallback to a default model if fit fails for any reason.

        :param X <np.ndarray>: feature vector.
        :param feature_colnames <list>: list of corresponding feature names.
        """
        if models_to_train is None:
            models_to_train = list(self.models.keys())
        self.n_fit_fail, self.n_fit_success = 0, 0
        for model in models_to_train:
            X_train = self.get_array_cols(feature_colnames, X, starts_with=model)
            try:
                self.models[model].fit(X_train)
                self.n_fit_success += 1
            except Exception as e:
                self.n_fit_fail += 1
                self.info(e)
                self.info(f'training failed for {model} at run_counter {self.runs_counter}, defaulting to hbos model.')
                self.models[model] = HBOS(contamination=self.contamination)
                self.models[model].fit(X_train)
            self.fitted_at[model] = self.runs_counter

    def predict(self):
        """Get latest data, make it into a feature vector, and get predictions for each available model.

        :return: (<dict>,<dict>) tuple of dictionaries, one for probability scores and the other for anomaly predictions.
        """
        # get recent data to predict on
        df_allmetrics = get_allmetrics(self.host, self.charts_in_scope, wide=True, sort_cols=True, protocol=self.protocol, numeric_only=True, float_size='float32')[self.expected_cols]
        if self.custom_models:
            df_allmetrics = self.add_custom_models_dims(df_allmetrics)
        self.df_allmetrics = self.df_allmetrics.append(df_allmetrics).ffill().tail((self.lags_n + self.smooth_n + self.diffs_n) * 2)

        # make feature vector
        X, feature_colnames = self.make_features(self.df_allmetrics.values, list(df_allmetrics.columns))
        data_probability, data_anomaly = self.try_predict(X, feature_colnames)

        return data_probability, data_anomaly

    def try_predict(self, X, feature_colnames):
        """Try make prediction and fall back to last known prediction if fails.

        :param X <np.ndarray>: feature vector.
        :param feature_colnames <list>: list of corresponding feature names.
        :return: (<dict>,<dict>) tuple of dictionaries, one for probability scores and the other for anomaly predictions.
        """
        data_probability, data_anomaly = {}, {}
        for model in self.fitted_at.keys():
            X_model = self.get_array_cols(feature_colnames, X, starts_with=model)
            try:
                data_probability[model + '_prob'] = np.nan_to_num(self.models[model].predict_proba(X_model)[-1][1]) * 100
                data_anomaly[model + '_anomaly'] = self.models[model].predict(X_model)[-1]
            except Exception as e:
                self.info(X_model)
                self.info(e)
                if model + '_prob' in self.data_latest:
                    self.info(f'prediction failed for {model} at run_counter {self.runs_counter}, using last prediction instead.')
                    data_probability[model + '_prob'] = self.data_latest[model + '_prob']
                    data_anomaly[model + '_anomaly'] = self.data_latest[model + '_anomaly']
                else:
                    self.info(f'prediction failed for {model} at run_counter {self.runs_counter}, skipping as no previous prediction.')
                    continue

        return data_probability, data_anomaly

    def get_data(self):

        if len(self.fitted_at) < len(self.models):
            self.train(models_to_train=[m for m in self.models if m not in self.fitted_at])
        elif self.runs_counter % self.train_every_n == 0:
            self.train()

        data_probability, data_anomaly = self.predict()
        data = {**data_probability, **data_anomaly}
        self.data_latest = data

        self.validate_charts('probability', data_probability, divisor=100)
        self.validate_charts('anomaly', data_anomaly)

        return data
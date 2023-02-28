# -*- coding: utf-8 -*-
# Description: anomalies netdata python.d module
# Author: andrewm4894
# SPDX-License-Identifier: GPL-3.0-or-later

import sys
import time
from datetime import datetime
import re
import warnings

import requests
import numpy as np
import pandas as pd
from netdata_pandas.data import get_data, get_allmetrics_async
from pyod.models.hbos import HBOS
from pyod.models.pca import PCA
from pyod.models.loda import LODA
from pyod.models.iforest import IForest
from pyod.models.cblof import CBLOF
from pyod.models.feature_bagging import FeatureBagging
from pyod.models.copod import COPOD
from sklearn.preprocessing import MinMaxScaler

from bases.FrameworkServices.SimpleService import SimpleService

# ignore some sklearn/numpy warnings that are ok
warnings.filterwarnings('ignore', r'All-NaN slice encountered')
warnings.filterwarnings('ignore', r'invalid value encountered in true_divide')
warnings.filterwarnings('ignore', r'divide by zero encountered in true_divide')
warnings.filterwarnings('ignore', r'invalid value encountered in subtract')

disabled_by_default = True

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
        self.basic_init()
        self.charts_init()
        self.custom_models_init()
        self.data_init()
        self.model_params_init()
        self.models_init()
        self.collected_dims = {'probability': set(), 'anomaly': set()}

    def check(self):
        if not (sys.version_info[0] >= 3 and sys.version_info[1] >= 6):
            self.error("anomalies collector only works with Python>=3.6")
        if len(self.host_charts_dict[self.host]) > 0:
            _ = get_allmetrics_async(host_charts_dict=self.host_charts_dict, protocol=self.protocol, user=self.username, pwd=self.password)
        return True

    def basic_init(self):
        """Perform some basic initialization.
        """
        self.order = ORDER
        self.definitions = CHARTS
        self.protocol = self.configuration.get('protocol', 'http')
        self.host = self.configuration.get('host', '127.0.0.1:19999')
        self.username = self.configuration.get('username', None)
        self.password = self.configuration.get('password', None)
        self.tls_verify = self.configuration.get('tls_verify', True)
        self.fitted_at = {}
        self.df_allmetrics = pd.DataFrame()
        self.last_train_at = 0
        self.include_average_prob = bool(self.configuration.get('include_average_prob', True))
        self.reinitialize_at_every_step = bool(self.configuration.get('reinitialize_at_every_step', False))

    def charts_init(self):
        """Do some initialisation of charts in scope related variables.
        """
        self.charts_regex = re.compile(self.configuration.get('charts_regex','None'))
        self.charts_available = [c for c in list(requests.get(f'{self.protocol}://{self.host}/api/v1/charts', verify=self.tls_verify).json().get('charts', {}).keys())]
        self.charts_in_scope = list(filter(self.charts_regex.match, self.charts_available))
        self.charts_to_exclude = self.configuration.get('charts_to_exclude', '').split(',')
        if len(self.charts_to_exclude) > 0:
            self.charts_in_scope = [c for c in self.charts_in_scope if c not in self.charts_to_exclude]

    def custom_models_init(self):
        """Perform initialization steps related to custom models.
        """
        self.custom_models = self.configuration.get('custom_models', None)
        self.custom_models_normalize = bool(self.configuration.get('custom_models_normalize', False))
        if self.custom_models:
            self.custom_models_names = [model['name'] for model in self.custom_models]
            self.custom_models_dims = [i for s in [model['dimensions'].split(',') for model in self.custom_models] for i in s]
            self.custom_models_dims = [dim if '::' in dim else f'{self.host}::{dim}' for dim in self.custom_models_dims]
            self.custom_models_charts = list(set([dim.split('|')[0].split('::')[1] for dim in self.custom_models_dims]))
            self.custom_models_hosts = list(set([dim.split('::')[0] for dim in self.custom_models_dims]))
            self.custom_models_host_charts_dict = {}
            for host in self.custom_models_hosts:
                self.custom_models_host_charts_dict[host] = list(set([dim.split('::')[1].split('|')[0] for dim in self.custom_models_dims if dim.startswith(host)]))
            self.custom_models_dims_renamed = [f"{model['name']}|{dim}" for model in self.custom_models for dim in model['dimensions'].split(',')]
            self.models_in_scope = list(set([f'{self.host}::{c}' for c in self.charts_in_scope] + self.custom_models_names))
            self.charts_in_scope = list(set(self.charts_in_scope + self.custom_models_charts))
            self.host_charts_dict = {self.host: self.charts_in_scope}
            for host in self.custom_models_host_charts_dict:
                if host not in self.host_charts_dict:
                    self.host_charts_dict[host] = self.custom_models_host_charts_dict[host]
                else:
                    for chart in self.custom_models_host_charts_dict[host]:
                        if chart not in self.host_charts_dict[host]:
                            self.host_charts_dict[host].extend(chart)
        else:
            self.models_in_scope = [f'{self.host}::{c}' for c in self.charts_in_scope]
            self.host_charts_dict = {self.host: self.charts_in_scope}
        self.model_display_names = {model: model.split('::')[1] if '::' in model else model for model in self.models_in_scope}
        #self.info(f'self.host_charts_dict (len={len(self.host_charts_dict[self.host])}): {self.host_charts_dict}')

    def data_init(self):
        """Initialize some empty data objects.
        """
        self.data_probability_latest = {f'{m}_prob': 0 for m in self.charts_in_scope}
        self.data_anomaly_latest = {f'{m}_anomaly': 0 for m in self.charts_in_scope}
        self.data_latest = {**self.data_probability_latest, **self.data_anomaly_latest}

    def model_params_init(self):
        """Model parameters initialisation.
        """
        self.train_max_n = self.configuration.get('train_max_n', 100000)
        self.train_n_secs = self.configuration.get('train_n_secs', 14400)
        self.offset_n_secs = self.configuration.get('offset_n_secs', 0)
        self.train_every_n = self.configuration.get('train_every_n', 1800)
        self.train_no_prediction_n = self.configuration.get('train_no_prediction_n', 10)
        self.initial_train_data_after = self.configuration.get('initial_train_data_after', 0)
        self.initial_train_data_before = self.configuration.get('initial_train_data_before', 0)
        self.contamination = self.configuration.get('contamination', 0.001)
        self.lags_n = {model: self.configuration.get('lags_n', 5) for model in self.models_in_scope}
        self.smooth_n = {model: self.configuration.get('smooth_n', 5) for model in self.models_in_scope}
        self.diffs_n = {model: self.configuration.get('diffs_n', 5) for model in self.models_in_scope}

    def models_init(self):
        """Models initialisation.
        """
        self.model = self.configuration.get('model', 'pca')
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
        self.custom_model_scalers = {model: MinMaxScaler() for model in self.models_in_scope}

    def model_init(self, model):
        """Model initialisation of a single model.
        """
        if self.model == 'pca':
            self.models[model] = PCA(contamination=self.contamination)
        elif self.model == 'loda':
            self.models[model] = LODA(contamination=self.contamination)
        elif self.model == 'iforest':
            self.models[model] = IForest(n_estimators=50, bootstrap=True, behaviour='new', contamination=self.contamination)
        elif self.model == 'cblof':
            self.models[model] = CBLOF(n_clusters=3, contamination=self.contamination)
        elif self.model == 'feature_bagging':
            self.models[model] = FeatureBagging(base_estimator=PCA(contamination=self.contamination), contamination=self.contamination)
        elif self.model == 'copod':
            self.models[model] = COPOD(contamination=self.contamination)
        elif self.model == 'hbos':
            self.models[model] = HBOS(contamination=self.contamination)
        else:
            self.models[model] = HBOS(contamination=self.contamination)
        self.custom_model_scalers[model] = MinMaxScaler()

    def reinitialize(self):
        """Reinitialize charts, models and data to a beginning state.
        """
        self.charts_init()
        self.custom_models_init()
        self.data_init()
        self.model_params_init()
        self.models_init()

    def save_data_latest(self, data, data_probability, data_anomaly):
        """Save the most recent data objects to be used if needed in the future.
        """
        self.data_latest = data
        self.data_probability_latest = data_probability
        self.data_anomaly_latest = data_anomaly

    def validate_charts(self, chart, data, algorithm='absolute', multiplier=1, divisor=1):
        """If dimension not in chart then add it.
        """
        for dim in data:
            if dim not in self.collected_dims[chart]:
                self.collected_dims[chart].add(dim)
                self.charts[chart].add_dimension([dim, dim, algorithm, multiplier, divisor])

        for dim in list(self.collected_dims[chart]):
            if dim not in data:
                self.collected_dims[chart].remove(dim)
                self.charts[chart].del_dimension(dim, hide=False)

    def add_custom_models_dims(self, df):
        """Given a df, select columns used by custom models, add custom model name as prefix, and append to df.

        :param df <pd.DataFrame>: dataframe to append new renamed columns to.
        :return: <pd.DataFrame> dataframe with additional columns added relating to the specified custom models.
        """
        df_custom = df[self.custom_models_dims].copy()
        df_custom.columns = self.custom_models_dims_renamed
        df = df.join(df_custom)

        return df

    def make_features(self, arr, train=False, model=None):
        """Take in numpy array and preprocess accordingly by taking diffs, smoothing and adding lags.

        :param arr <np.ndarray>: numpy array we want to make features from.
        :param train <bool>: True if making features for training, in which case need to fit_transform scaler and maybe sample train_max_n.
        :param model <str>: model to make features for.
        :return: <np.ndarray> transformed numpy array.
        """

        def lag(arr, n):
            res = np.empty_like(arr)
            res[:n] = np.nan
            res[n:] = arr[:-n]

            return res

        arr = np.nan_to_num(arr)

        diffs_n = self.diffs_n[model]
        smooth_n = self.smooth_n[model]
        lags_n = self.lags_n[model]

        if self.custom_models_normalize and model in self.custom_models_names:
            if train:
                arr = self.custom_model_scalers[model].fit_transform(arr)
            else:
                arr = self.custom_model_scalers[model].transform(arr)

        if diffs_n > 0:
            arr = np.diff(arr, diffs_n, axis=0)
            arr = arr[~np.isnan(arr).any(axis=1)]

        if smooth_n > 1:
            arr = np.cumsum(arr, axis=0, dtype=float)
            arr[smooth_n:] = arr[smooth_n:] - arr[:-smooth_n]
            arr = arr[smooth_n - 1:] / smooth_n
            arr = arr[~np.isnan(arr).any(axis=1)]

        if lags_n > 0:
            arr_orig = np.copy(arr)
            for lag_n in range(1, lags_n + 1):
                arr = np.concatenate((arr, lag(arr_orig, lag_n)), axis=1)
            arr = arr[~np.isnan(arr).any(axis=1)]

        if train:
            if len(arr) > self.train_max_n:
                arr = arr[np.random.randint(arr.shape[0], size=self.train_max_n), :]

        arr = np.nan_to_num(arr)

        return arr

    def train(self, models_to_train=None, train_data_after=0, train_data_before=0):
        """Pull required training data and train a model for each specified model.

        :param models_to_train <list>: list of models to train on.
        :param train_data_after <int>: integer timestamp for start of train data.
        :param train_data_before <int>: integer timestamp for end of train data.
        """
        now = datetime.now().timestamp()
        if train_data_after > 0 and train_data_before > 0:
            before = train_data_before
            after = train_data_after
        else:
            before = int(now) - self.offset_n_secs
            after =  before - self.train_n_secs

        # get training data
        df_train = get_data(
            host_charts_dict=self.host_charts_dict, host_prefix=True, host_sep='::', after=after, before=before,
            sort_cols=True, numeric_only=True, protocol=self.protocol, float_size='float32', user=self.username, pwd=self.password,
            verify=self.tls_verify
        ).ffill()
        if self.custom_models:
            df_train = self.add_custom_models_dims(df_train)

        # train model
        self.try_fit(df_train, models_to_train=models_to_train)
        self.info(f'training complete in {round(time.time() - now, 2)} seconds (runs_counter={self.runs_counter}, model={self.model}, train_n_secs={self.train_n_secs}, models={len(self.fitted_at)}, n_fit_success={self.n_fit_success}, n_fit_fails={self.n_fit_fail}, after={after}, before={before}).')
        self.last_train_at = self.runs_counter

    def try_fit(self, df_train, models_to_train=None):
        """Try fit each model and try to fallback to a default model if fit fails for any reason.

        :param df_train <pd.DataFrame>: data to train on.
        :param models_to_train <list>: list of models to train.
        """
        if models_to_train is None:
            models_to_train = list(self.models.keys())
        self.n_fit_fail, self.n_fit_success = 0, 0
        for model in models_to_train:
            if model not in self.models:
                self.model_init(model)
            X_train = self.make_features(
                df_train[df_train.columns[df_train.columns.str.startswith(f'{model}|')]].values,
                train=True, model=model)
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
        df_allmetrics = get_allmetrics_async(
            host_charts_dict=self.host_charts_dict, host_prefix=True, host_sep='::', wide=True, sort_cols=True,
            protocol=self.protocol, numeric_only=True, float_size='float32', user=self.username, pwd=self.password
            )
        if self.custom_models:
            df_allmetrics = self.add_custom_models_dims(df_allmetrics)
        self.df_allmetrics = self.df_allmetrics.append(df_allmetrics).ffill().tail((max(self.lags_n.values()) + max(self.smooth_n.values()) + max(self.diffs_n.values())) * 2)

        # get predictions
        data_probability, data_anomaly = self.try_predict()

        return data_probability, data_anomaly

    def try_predict(self):
        """Try make prediction and fall back to last known prediction if fails.

        :return: (<dict>,<dict>) tuple of dictionaries, one for probability scores and the other for anomaly predictions.
        """
        data_probability, data_anomaly = {}, {}
        for model in self.fitted_at.keys():
            model_display_name = self.model_display_names[model]
            try:
                X_model = np.nan_to_num(
                    self.make_features(
                        self.df_allmetrics[self.df_allmetrics.columns[self.df_allmetrics.columns.str.startswith(f'{model}|')]].values,
                        model=model
                    )[-1,:].reshape(1, -1)
                )
                data_probability[model_display_name + '_prob'] = np.nan_to_num(self.models[model].predict_proba(X_model)[-1][1]) * 10000
                data_anomaly[model_display_name + '_anomaly'] = self.models[model].predict(X_model)[-1]
            except Exception as _:
                #self.info(e)
                if model_display_name + '_prob' in self.data_latest:
                    #self.info(f'prediction failed for {model} at run_counter {self.runs_counter}, using last prediction instead.')
                    data_probability[model_display_name + '_prob'] = self.data_latest[model_display_name + '_prob']
                    data_anomaly[model_display_name + '_anomaly'] = self.data_latest[model_display_name + '_anomaly']
                else:
                    #self.info(f'prediction failed for {model} at run_counter {self.runs_counter}, skipping as no previous prediction.')
                    continue

        return data_probability, data_anomaly

    def get_data(self):

        # initialize to what's available right now
        if self.reinitialize_at_every_step or len(self.host_charts_dict[self.host]) == 0:
            self.charts_init()
            self.custom_models_init()
            self.model_params_init()

        # if not all models have been trained then train those we need to
        if len(self.fitted_at) < len(self.models_in_scope):
            self.train(
                models_to_train=[m for m in self.models_in_scope if m not in self.fitted_at],
                train_data_after=self.initial_train_data_after,
                train_data_before=self.initial_train_data_before
            )
        # retrain all models as per schedule from config
        elif self.train_every_n > 0 and self.runs_counter % self.train_every_n == 0:
            self.reinitialize()
            self.train()

        # roll forward previous predictions around a training step to avoid the possibility of having the training itself trigger an anomaly
        if (self.runs_counter - self.last_train_at) <= self.train_no_prediction_n:
            data_probability = self.data_probability_latest
            data_anomaly = self.data_anomaly_latest
        else:
            data_probability, data_anomaly = self.predict()
            if self.include_average_prob:
                average_prob = np.mean(list(data_probability.values()))
                data_probability['average_prob'] = 0 if np.isnan(average_prob) else average_prob
        
        data = {**data_probability, **data_anomaly}

        self.validate_charts('probability', data_probability, divisor=100)
        self.validate_charts('anomaly', data_anomaly)

        self.save_data_latest(data, data_probability, data_anomaly)

        #self.info(f'len(data)={len(data)}')
        #self.info(f'data')

        return data

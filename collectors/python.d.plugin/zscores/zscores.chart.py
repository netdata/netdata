# -*- coding: utf-8 -*-
# Description: zscores netdata python.d module
# Author: andrewm4894
# SPDX-License-Identifier: GPL-3.0-or-later

from datetime import datetime
import re

import requests
import numpy as np
import pandas as pd

from bases.FrameworkServices.SimpleService import SimpleService
from netdata_pandas.data import get_data, get_allmetrics

priority = 60000
update_every = 5
disabled_by_default = True

ORDER = [
    'z',
    '3stddev'
]

CHARTS = {
    'z': {
        'options': ['z', 'Z Score', 'z', 'Z Score', 'zscores.z', 'line'],
        'lines': []
    },
    '3stddev': {
        'options': ['3stddev', 'Z Score >3', 'count', '3 Stddev', 'zscores.3stddev', 'stacked'],
        'lines': []
    },
}


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.host = self.configuration.get('host', '127.0.0.1:19999')
        self.charts_regex = re.compile(self.configuration.get('charts_regex', 'system.*'))
        self.charts_to_exclude = self.configuration.get('charts_to_exclude', '').split(',')
        self.charts_in_scope = [
            c for c in
            list(filter(self.charts_regex.match,
                        requests.get(f'http://{self.host}/api/v1/charts').json()['charts'].keys()))
            if c not in self.charts_to_exclude
        ]
        self.train_secs = self.configuration.get('train_secs', 14400)
        self.offset_secs = self.configuration.get('offset_secs', 300)
        self.train_every_n = self.configuration.get('train_every_n', 900)
        self.z_smooth_n = self.configuration.get('z_smooth_n', 15)
        self.z_clip = self.configuration.get('z_clip', 10)
        self.z_abs = bool(self.configuration.get('z_abs', True))
        self.burn_in = self.configuration.get('burn_in', 2)
        self.mode = self.configuration.get('mode', 'per_chart')
        self.per_chart_agg = self.configuration.get('per_chart_agg', 'mean')
        self.order = ORDER
        self.definitions = CHARTS
        self.collected_dims = {'z': set(), '3stddev': set()}
        self.df_mean = pd.DataFrame()
        self.df_std = pd.DataFrame()
        self.df_z_history = pd.DataFrame()

    def check(self):
        _ = get_allmetrics(self.host, self.charts_in_scope, wide=True, col_sep='.')
        return True

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

    def train_model(self):
        """Calculate the mean and stddev for all relevant metrics and store them for use in calulcating zscore at each timestep.
        """
        before = int(datetime.now().timestamp()) - self.offset_secs
        after = before - self.train_secs

        self.df_mean = get_data(
            self.host, self.charts_in_scope, after, before, points=10, group='average', col_sep='.'
        ).mean().to_frame().rename(columns={0: "mean"})

        self.df_std = get_data(
            self.host, self.charts_in_scope, after, before, points=10, group='stddev', col_sep='.'
        ).mean().to_frame().rename(columns={0: "std"})

    def create_data(self, df_allmetrics):
        """Use x, mean, stddev to generate z scores and 3stddev flags via some pandas manipulation.
        Returning two dictionaries of dimensions and measures, one for each chart.

        :param df_allmetrics <pd.DataFrame>: pandas dataframe with latest data from api/v1/allmetrics.
        :return: (<dict>,<dict>) tuple of dictionaries, one for  zscores and the other for a flag if abs(z)>3.
        """
        # calculate clipped z score for each available metric
        df_z = pd.concat([self.df_mean, self.df_std, df_allmetrics], axis=1, join='inner')
        df_z['z'] = ((df_z['value'] - df_z['mean']) / df_z['std']).clip(-self.z_clip, self.z_clip).fillna(0) * 100
        if self.z_abs:
            df_z['z'] = df_z['z'].abs()

        # append last z_smooth_n rows of zscores to history table in wide format
        self.df_z_history = self.df_z_history.append(
            df_z[['z']].reset_index().pivot_table(values='z', columns='index'), sort=True
        ).tail(self.z_smooth_n)

        # get average zscore for last z_smooth_n for each metric
        df_z_smooth = self.df_z_history.melt(value_name='z').groupby('index')['z'].mean().to_frame()
        df_z_smooth['3stddev'] = np.where(abs(df_z_smooth['z']) > 300, 1, 0)
        data_z = df_z_smooth['z'].add_suffix('_z').to_dict()

        # aggregate to chart level if specified
        if self.mode == 'per_chart':
            df_z_smooth['chart'] = ['.'.join(x[0:2]) + '_z' for x in df_z_smooth.index.str.split('.').to_list()]
            if self.per_chart_agg == 'absmax':
                data_z = \
                list(df_z_smooth.groupby('chart').agg({'z': lambda x: max(x, key=abs)})['z'].to_dict().values())[0]
            else:
                data_z = list(df_z_smooth.groupby('chart').agg({'z': [self.per_chart_agg]})['z'].to_dict().values())[0]

        data_3stddev = {}
        for k in data_z:
            data_3stddev[k.replace('_z', '')] = 1 if abs(data_z[k]) > 300 else 0

        return data_z, data_3stddev

    def get_data(self):

        if self.runs_counter <= self.burn_in or self.runs_counter % self.train_every_n == 0:
            self.train_model()

        data_z, data_3stddev = self.create_data(
            get_allmetrics(self.host, self.charts_in_scope, wide=True, col_sep='.').transpose())
        data = {**data_z, **data_3stddev}

        self.validate_charts('z', data_z, divisor=100)
        self.validate_charts('3stddev', data_3stddev)

        return data

# -*- coding: utf-8 -*-
# Description: zscores netdata python.d module
# Author: andrewm4894
# SPDX-License-Identifier: GPL-3.0-or-later

from datetime import datetime
from random import SystemRandom

import requests
import numpy as np
import pandas as pd

from bases.FrameworkServices.SimpleService import SimpleService
from netdata_pandas.data import get_data, get_allmetrics


priority = 50
update_every = 1

DEFAULT_HOST = '127.0.0.1:19999'
DEFAULT_CHARTS_IN_SCOPE = 'system.cpu,system.load,system.io,system.pgpgio,system.ram,system.net,system.ip,system.ipv6,system.processes,system.ctxt,system.idlejitter,system.intr,system.softirqs,system.softnet_stat'
DEFAULT_TRAIN_SECS = 60*60*1 # use last 1 hour to work out the mean and sigma for the zscore
DEFAULT_OFFSET_SECS = 60*5 # ignore last 5 minutes of data when calculating the mean and sigma
DEFAULT_TRAIN_EVERY_N = 60*5 # recalculate mean and sigma every 5 minutes
DEFAULT_Z_SMOOTH_N = 30 # take a rolling average of the last 30 zscore values to reduce sensitivity to temporary 'spikes'
DEFAULT_Z_CLIP = 10 # cap each zscore at 10 so as to avoid really large individual zscores swamping any rolling average
DEFAULT_BURN_IN = 20 # on startup of the collector continually update the mean and sigma incase any gaps or inital calculations fail to return
DEFAULT_MODE = 'per_chart' # 'per_chart' means individual dimension level smoothed zscores will be averaged again to one zscore per chart per time step

ORDER = [
    'zscores',
    'zscores_3sigma'
]

CHARTS = {
    'zscores': {
        'options': [None, 'Z Scores', 'zscores', 'zscore', 'zscores.zscores', 'line'],
        'lines': []
    },
    'zscores_3sigma': {
        'options': [None, 'Z Scores >3 Sigma', '3sig count', 'zscores', 'zscores.zscores_3sigma', 'stacked'],
        'lines': []
    },
}


class Service(SimpleService):
    
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.host = self.configuration.get('host', DEFAULT_HOST)
        self.charts_in_scope = self.configuration.get('charts_in_scope', DEFAULT_CHARTS_IN_SCOPE).split(',')
        self.train_secs = self.configuration.get('train_secs', DEFAULT_TRAIN_SECS)
        self.offset_secs = self.configuration.get('offset_secs', DEFAULT_OFFSET_SECS)
        self.train_every_n = self.configuration.get('train_every_n', DEFAULT_TRAIN_EVERY_N)
        self.z_smooth_n = self.configuration.get('z_smooth_n', DEFAULT_Z_SMOOTH_N) 
        self.z_clip = self.configuration.get('z_clip', DEFAULT_Z_CLIP)
        self.burn_in = self.configuration.get('burn_in', DEFAULT_BURN_IN) 
        self.mode = self.configuration.get('mode', DEFAULT_MODE) 
        self.order = ORDER
        self.definitions = CHARTS
        self.random = SystemRandom()
        self.df_mean = pd.DataFrame()
        self.df_std = pd.DataFrame()
        self.df_z_history = pd.DataFrame()

    @staticmethod
    def check():
        return True

    def train_model(self):
        """Calculate the mean and sigma for all relevant metrics and 
        store them for use in calulcating z score at each timestep. 
        """

        now = int(datetime.now().timestamp())
        after = now - self.offset_secs - self.train_secs
        before = now - self.offset_secs

        # get means
        self.df_mean = get_data(
            hosts=self.host, charts=self.charts_in_scope, after=after, 
            before=before, points=1, group='average', col_sep='.'
            ).transpose()
        self.df_mean.columns = ['mean']

        # get sigmas
        self.df_std = get_data(
            hosts=self.host, charts=self.charts_in_scope, after=after, 
            before=before, points=1, group='stddev', col_sep='.'
            ).mean().to_frame()
        self.df_std.columns = ['std']

    def create_data_dicts(self, df_allmetrics):
        """Use x, mean, sigma to generate z scores and 3sig flags via some pandas manipulation.
        Returning two dictionarier of dimensions and measures, one for each chart.
        """

        # calculate clipped z score for each available metric
        df_z = pd.concat([self.df_mean, self.df_std, df_allmetrics], axis=1, join='inner')
        df_z['z'] = ((df_z['value'] - df_z['mean']) / df_z['std']).clip(lower=-self.z_clip, upper=self.z_clip).fillna(0)

        # append last z_smooth_n rows of zscores to history table
        df_z_wide = df_z[['z']].reset_index().pivot_table(values='z', columns='index')

        self.df_z_history = self.df_z_history.append(df_z_wide, sort=True).tail(self.z_smooth_n)

        # get average zscore for last z_smooth_n for each metric
        df_z_smooth = (df_z_wide.melt(value_name='z').groupby('index')['z'].mean() * 100).to_frame()
        df_z_smooth['3sig'] = np.where(abs(df_z_smooth['z']) > 300, 1, 0)
        
        # create data dict for z scores (with keys renamed)
        dim_names_z = ['.'.join(x.split('.')) + '_z' for x in df_z_smooth.index]
        df_z_smooth.index = dim_names_z
        data_dict_z = df_z_smooth['z'].to_dict()
        
        # create data dict for 3sig flags (with keys renamed)
        dim_names_3sig = [x[:-2] + '_3sig' for x in df_z_smooth.index]
        df_z_smooth.index = dim_names_3sig
        data_dict_3sig = df_z_smooth['3sig'].to_dict()

        # average to chart level if specified
        if self.mode == 'per_chart':

            df_z_chart = pd.DataFrame.from_dict(data_dict_z, orient='index').reset_index()
            df_z_chart.columns = ['dim', 'z']
            df_z_chart['chart'] = ['.'.join(x[0:2]) + '_z' for x in df_z_chart['dim'].str.split('.').to_list()]

            data_dict_z = df_z_chart.groupby('chart')['z'].mean().to_dict()

            data_dict_3sig = {}
            for k in data_dict_z:
                data_dict_3sig[k.replace('_z','_3sig')] = 1 if abs(data_dict_z[k]) > 300 else 0

        return data_dict_z, data_dict_3sig

    def get_data(self):

        # train model if needed
        if self.runs_counter <= self.burn_in or self.runs_counter % self.train_every_n == 0:
            self.train_model()

        # get latest data
        df_allmetrics = get_allmetrics(
            host=self.host, 
            charts=self.charts_in_scope, 
            wide=True, 
            col_sep='.'
            ).transpose()

        # create data dicts
        data_dict_z, data_dict_3sig = self.create_data_dicts(df_allmetrics)
        data = {**data_dict_z, **data_dict_3sig}

        # validate relevant dims in charts
        for dim in data_dict_z:
            if dim not in self.charts['zscores']:
                self.charts['zscores'].add_dimension([dim, dim, 'absolute', 1, 100])
        for dim in data_dict_3sig:
            if dim not in self.charts['zscores_3sigma']:
                self.charts['zscores_3sigma'].add_dimension([dim, dim, 'absolute', 1, 1])

        return data

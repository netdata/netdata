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


priority = 2
update_every = 1

HOST = '127.0.0.1:19999'
#HOST = 'london.my-netdata.io'
CHARTS_IN_SCOPE = [
    'system.cpu', 'system.load', 'system.io', 'system.pgpgio', 'system.ram', 'system.net', 'system.ip', 'system.ipv6',
    'system.processes', 'system.ctxt', 'system.idlejitter', 'system.intr', 'system.softirqs', 'system.softnet_stat'
]

TRAIN_N_SECS = 60*2
OFFSET_N_SECS = 60
TRAIN_EVERY_N = 60
Z_SMOOTH_N = 5
Z_SCORE_CLIP = 10

ORDER = [
    'zscores',
    'zscores_3sigma'
]

CHARTS = {
    'zscores': {
        'options': [None, 'Z Scores', 'name.chart', 'zscores', 'zscores.zscores', 'line'],
        'lines': []
    },
    'zscores_3sigma': {
        'options': [None, 'Z Scores >3 Sigma', 'name.chart', 'zscores', 'zscores.zscores_3sigma', 'stacked'],
        'lines': []
    },
}


class Service(SimpleService):
    
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.random = SystemRandom()
        self.df_mean = pd.DataFrame()
        self.df_std = pd.DataFrame()
        self.df_z_history = pd.DataFrame()

    @staticmethod
    def check():
        return True

    def get_data(self):

        now = int(datetime.now().timestamp())
        after = now - OFFSET_N_SECS - TRAIN_N_SECS
        before = now - OFFSET_N_SECS
        #self.debug(f'now={now}')

        if self.runs_counter <= 10 or self.runs_counter % TRAIN_EVERY_N == 0:

            #self.debug(f'begin training (runs_counter={self.runs_counter})')
            
            self.df_mean = get_data(HOST, charts=CHARTS_IN_SCOPE, after=after, before=before, points=1, group='average', col_sep='.')
            self.df_mean = self.df_mean.transpose()
            self.df_mean.columns = ['mean']
            #self.debug('self.df_mean')
            #self.debug(self.df_mean)

            self.df_std = get_data(HOST, charts=CHARTS_IN_SCOPE, after=after, before=before, points=1, group='stddev', col_sep='.')
            self.df_std = self.df_std.transpose()
            self.df_std.columns = ['std']
            self.df_std = self.df_std[self.df_std['std']>0]
            #self.debug('self.df_std')
            #self.debug(self.df_std)

        df_allmetrics = get_allmetrics(HOST, charts=CHARTS_IN_SCOPE, wide=True, col_sep='.').transpose()
        #self.debug(f'df_allmetrics.shape={df_allmetrics.shape}')

        df_z = pd.concat([self.df_mean, self.df_std, df_allmetrics], axis=1, join='inner')
        df_z['z'] = np.where(df_z['std'] > 0, (df_z['value'] - df_z['mean']) / df_z['std'], 0)
        df_z['z'] = df_z['z'].fillna(0).clip(lower=-Z_SCORE_CLIP, upper=Z_SCORE_CLIP)
        df_z_wide = df_z[['z']].reset_index().pivot_table(values='z', columns='index')

        self.df_z_history = self.df_z_history.append(df_z_wide).tail(Z_SMOOTH_N)

        df_z_smooth = self.df_z_history.reset_index().groupby('index')[['z']].mean() * 100
        df_z_smooth['3sig'] = np.where(abs(df_z_smooth['z']) > 300, 1, 0)
        
        df_z_smooth.index = ['.'.join(reversed(x.split('.'))) + '_z' for x in df_z_smooth.index]
        data_dict_z = df_z_smooth['z'].to_dict()
        
        df_z_smooth.index = [x[:-2] + '_3sig' for x in df_z_smooth.index]
        data_dict_3sig = df_z_smooth['3sig'].to_dict()

        data = {**data_dict_z, **data_dict_3sig}
        #self.debug('data_dict')
        #self.debug(data_dict)

        for dim in data_dict_z:
            if dim not in self.charts['zscores']:
                self.charts['zscores'].add_dimension([dim, dim, 'absolute', 1, 100])
        
        for dim in data_dict_3sig:
            if dim not in self.charts['zscores_3sigma']:
                self.charts['zscores_3sigma'].add_dimension([dim, dim, 'absolute', 1, 1])

        return data

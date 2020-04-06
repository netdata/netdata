# -*- coding: utf-8 -*-
# Description: correlations netdata python.d module
# Author: andrewm4894
# SPDX-License-Identifier: GPL-3.0-or-later


import requests
import pandas as pd
from bases.FrameworkServices.SimpleService import SimpleService

priority = 2

HOST = '127.0.0.1:19999'
CHARTS_IN_SCOPE = [
    'system.cpu', 'system.load', 'system.io', 'system.pgpgio', 'system.ram', 'system.net', 'system.ip', 'system.ipv6',
    'system.processes', 'system.ctxt', 'system.idlejitter', 'system.intr', 'system.softirqs', 'system.softnet_stat'
]
TRAIN_MAX_N = 60*5
CORR_DIFF_THOLD = 0.5

ORDER = [
    'metric_correlations',
    'metric_correlation_changes'
]

CHARTS = {
    'metric_correlations': {
        'options': [None, 'Metric Correlations', '(var1,var2)', 'metric correlations', 'correlations.correlations',
                    'lines'],
        'lines': []
    },
    'metric_correlation_changes': {
        'options': [
            None, 'Metric Correlation Changes', '(var1,var2)', 'metric correlation changes', 'correlations.changes',
            'stacked'],
        'lines': []
    },
}


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.data = []
        self.charts_in_scope = CHARTS_IN_SCOPE
        self.train_max_n = TRAIN_MAX_N
        self.host = HOST
        self.correlations = dict()
        self.corr_diff_thold = CORR_DIFF_THOLD


    @staticmethod
    def check():
        return True

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

    def get_allmetrics(self) -> list:
        """
        Hits the allmetrics endpoint on `host` filters for `charts` of interest and saves data into a list
        :param host: host to pull data from <str>
        :param charts: charts to filter to <list>
        :return: list of lists where each element is a metric from allmetrics <list>
        """
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
        self.append_data(latest_observations)

        # limit size of data maintained to last n
        self.data = self.data[-self.train_max_n:]

        # pull data into a pandas df
        df = self.data_to_df(self.data)

        # get correlation matrix
        df = df.corr()
        df = df.rename_axis("var1", axis="index")
        df = df.rename_axis("var2", axis="columns")
        df = df.stack().to_frame().reset_index().rename(columns={0: 'value'})
        df['variable'] = df['var1'] + '__' + df['var2']
        df = df[df['var1'] != df['var2']]
        df = df[['variable', 'value']]
        df['idx'] = 1
        df = df.pivot(index='idx', columns='variable', values='value')

        # add to chart data
        for col in df.columns:
            correlation = df[col].values[0]
            dimension_id = col.replace('system.', '').replace('__', ' , ')
            dimension_id = '({})'.format(dimension_id)
            dimension_id_reversed = dimension_id.replace(' ', '').replace('(', '').replace(')', '').split(',')
            dimension_id_reversed = "({} , {})".format(dimension_id_reversed[1], dimension_id_reversed[0])
            if dimension_id_reversed in data:
                self.debug('skipping {} as {} already in data'.format(dimension_id, dimension_id_reversed))
            else:
                dimension_id_flag = '{} flag'.format(dimension_id)
                if dimension_id in self.correlations:
                    correlation_diff = correlation - self.correlations[dimension_id]
                else:
                    correlation_diff = 0
                self.debug('dimension_id={}, correlation={}, correlation_diff={}'.format(
                    dimension_id, correlation, correlation_diff))
                # update correlation in self
                self.correlations[dimension_id] = correlation
                if dimension_id not in self.charts['metric_correlations']:
                    self.charts['metric_correlations'].add_dimension([dimension_id, dimension_id, 'absolute', 1, 100])
                data[dimension_id] = correlation * 100
                if abs(correlation_diff) > self.corr_diff_thold:
                    if dimension_id_flag not in self.charts['metric_correlation_changes']:
                        self.charts['metric_correlation_changes'].add_dimension(
                            [dimension_id_flag, dimension_id_flag, 'absolute', 1, 1]
                        )
                    data[dimension_id_flag] = 1

        return data















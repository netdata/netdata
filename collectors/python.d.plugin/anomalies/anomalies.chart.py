# -*- coding: utf-8 -*-
# Description: anomalies netdata python.d module
# Author: andrewm4894
# SPDX-License-Identifier: GPL-3.0-or-later

from random import SystemRandom, randint

from bases.FrameworkServices.SimpleService import SimpleService

import requests
import numpy as np
import pandas as pd


priority = 1

ORDER = [
    'cpu',
    'load',
    'ram',
    'io',
    'net',
    'scores',
    'anomalies',
]

CHARTS = {
    'cpu': {
        'options': [None, 'CPU Anomaly Scores', 'value', 'cpu', 'anomalies.cpu', 'line'],
        'lines': [
            ['cpu_expected', 'cpu expected', 'absolute', 1, 100],
            ['cpu_actual', 'cpu actual', 'absolute', 1, 100],
            ['cpu_error', 'error', 'absolute', 1, 100]
        ]
    },
    'load': {
        'options': [None, 'Load Anomaly Scores', 'value', 'load', 'anomalies.load', 'line'],
        'lines': [
            ['load_expected', 'load expected', 'absolute', 1, 100],
            ['load_actual', 'load actual', 'absolute', 1, 100],
            ['load_error', 'error', 'absolute', 1, 100]
        ]
    },
    'ram': {
        'options': [None, 'RAM Anomaly Scores', 'value', 'ram', 'anomalies.ram', 'line'],
        'lines': [
            ['ram_expected', 'ram expected', 'absolute', 1, 100],
            ['ram_actual', 'ram actual', 'absolute', 1, 100],
            ['ram_error', 'error', 'absolute', 1, 100]
        ]
    },
    'io': {
        'options': [None, 'IO Anomaly Scores', 'value', 'io', 'anomalies.io', 'line'],
        'lines': [
            ['io_expected', 'io expected', 'absolute', 1, 100],
            ['io_actual', 'io actual', 'absolute', 1, 100],
            ['io_error', 'error', 'absolute', 1, 100]
        ]
    },
    'net': {
        'options': [None, 'Network Anomaly Scores', 'value', 'net', 'anomalies.net', 'line'],
        'lines': [
            ['net_expected', 'net expected', 'absolute', 1, 100],
            ['net_actual', 'net actual', 'absolute', 1, 100],
            ['net_error', 'error', 'absolute', 1, 100]
        ]
    },
    'scores': {
        'options': [None, 'All Anomaly Scores', 'value', 'scores', 'anomalies.scores', 'line'],
        'lines': [
            ['cpu_score', 'cpu score', 'absolute', 1, 100],
            ['load_score', 'load score', 'absolute', 1, 100],
            ['ram_score', 'ram score', 'absolute', 1, 100],
            ['io_score', 'io score', 'absolute', 1, 100],
            ['net_score', 'net score', 'absolute', 1, 100]
        ]
    },
    'anomalies': {
        'options': [None, 'All Anomaly Events', 'is_anomaly', 'anomalies', 'anomalies.anomalies', 'line'],
        'lines': [
            ['cpu_anomaly', 'cpu anomaly', 'absolute', 1, 1],
            ['load_anomaly', 'load anomaly', 'absolute', 1, 1],
            ['ram_anomaly', 'ram anomaly', 'absolute', 1, 1],
            ['io_anomaly', 'io anomaly', 'absolute', 1, 1],
            ['net_anomaly', 'net anomaly', 'absolute', 1, 1]
        ]
    },
}

HOST_PORT = '127.0.0.1:19999'


def get_raw_data(host=None):

    if host is None:
        host = HOST_PORT

    data = dict()

    for chart in list(set(CHARTS.keys()) - set(['scores', 'anomalies'])):

        # get data
        after = -10
        url = f'http://{host}/api/v1/data?chart=system.{chart}&after={after}&format=json'
        response = requests.get(url)
        response_json = response.json()
        raw_data = response_json['data']
        df = pd.DataFrame(raw_data, columns=response_json['labels'])
        df = df.set_index('time')

        # create data for lines
        actual = np.mean(abs(df.values))
        rand_error_pct = randint(-20, 20) / 100
        expected = actual + (rand_error_pct * actual)
        error = abs(actual - expected)
        error_pct = error / actual
        anomalies = 1.0 if error_pct > 0.1 else 0.0

        # add data
        data[f'{chart}_expected'] = (expected * 100)
        data[f'{chart}_actual'] = (actual * 100)
        data[f'{chart}_error'] = (error * 100)
        data[f'{chart}_score'] = (error_pct * 100)
        data[f'{chart}_anomaly'] = anomalies

    return data


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.random = SystemRandom()

    @staticmethod
    def check():
        return True

    def get_data(self):

        data = get_raw_data()

        return data


#%%

#data = get_raw_data('london.my-netdata.io')
#print(data)

#%%

#%%

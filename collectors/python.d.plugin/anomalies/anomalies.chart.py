# -*- coding: utf-8 -*-
# Description: anomalies netdata python.d module
# Author: andrewm4894
# SPDX-License-Identifier: GPL-3.0-or-later

from random import SystemRandom, randint

from bases.FrameworkServices.SimpleService import SimpleService

import requests
import numpy as np


priority = 1

ORDER = [
    'cpu',
    'load',
    'io',
    'net',
    'scores',
    'anomalies',
]

CHARTS = {
    'cpu': {
        'options': [None, 'CPU Anomaly Scores', 'value', 'cpu', 'anomalies.cpu', 'line'],
        'lines': [
            ['cpu_expected', None, 'absolute', 1, 100],
            ['cpu_actual', None, 'absolute', 1, 100],
            ['cpu_error', None, 'absolute', 1, 100]
        ]
    },
    'load': {
        'options': [None, 'Load Anomaly Scores', 'value', 'load', 'anomalies.load', 'line'],
        'lines': [
            ['load_expected'], ['load_actual'], ['load_error']
        ]
    },
    'io': {
        'options': [None, 'IO Anomaly Scores', 'value', 'io', 'anomalies.io', 'line'],
        'lines': [
            ['io_expected'], ['io_actual'], ['io_error']
        ]
    },
    'net': {
        'options': [None, 'Network Anomaly Scores', 'value', 'net', 'anomalies.net', 'line'],
        'lines': [
            ['net_expected'], ['net_actual'], ['net_error']
        ]
    },
    'scores': {
        'options': [None, 'All Anomaly Scores', 'value', 'scores', 'anomalies.scores', 'line'],
        'lines': [
            ['cpu_score'], ['load_score'], ['io_score'], ['net_score']
        ]
    },
    'anomalies': {
        'options': [None, 'All Anomaly Events', 'is_anomaly', 'anomalies', 'anomalies.anomalies', 'line'],
        'lines': [
            ['cpu_anomaly'], ['load_anomaly'], ['io_anomaly'], ['net_anomaly']
        ]
    },
}

HOST_PORT = '127.0.0.1:19999'
#HOST_PORT = 'london.my-netdata.io'

def tmp_get_data(host=None):

    if host is None:
        host = HOST_PORT

    data = dict()

    for chart in list(set(CHARTS.keys()) - set(['scores', 'anomalies'])):
        after = -10
        url = f'http://{host}/api/v1/data?chart=system.{chart}&after={after}&format=json'
        response = requests.get(url)
        raw_data = response.json()['data'][0][1:]

        actual = np.mean(raw_data)
        rand_error_pct = randint(-20, 20) / 100
        expected = actual + (rand_error_pct * actual)
        error = abs(actual - expected)
        error_pct = error / actual
        anomalies = 1.0 if error_pct > 0.1 else 0.0

        data[f'{chart}_expected'] = expected * 1000
        data[f'{chart}_actual'] = actual * 1000
        data[f'{chart}_error'] = error * 1000
        data[f'{chart}_score'] = error * 1000
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

        data = tmp_get_data()

        return data

#%%

#data = tmp_get_data('london.my-netdata.io')
#print(data)

#%%

#%%

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
    'ip',
    'ipv6',
    'processes',
    'intr',
    'scores',
    'anomalies',
]

CHARTS = {
    'cpu': {
        'options': [None, 'CPU Anomaly Scores', 'value', 'cpu', 'anomalies.cpu', 'line'],
        'lines': [
            ['cpu_expected', 'expected', 'absolute', 1, 1000],
            ['cpu_actual', 'cpu', 'absolute', 1, 1000],
            ['cpu_error', 'error', 'absolute', 1, 1000]
        ]
    },
    'load': {
        'options': [None, 'Load Anomaly Scores', 'value', 'load', 'anomalies.load', 'line'],
        'lines': [
            ['load_expected', 'expected', 'absolute', 1, 1000],
            ['load_actual', 'load', 'absolute', 1, 1000],
            ['load_error', 'error', 'absolute', 1, 1000]
        ]
    },
    'ram': {
        'options': [None, 'RAM Anomaly Scores', 'value', 'ram', 'anomalies.ram', 'line'],
        'lines': [
            ['ram_expected', 'expected', 'absolute', 1, 1000],
            ['ram_actual', 'ram', 'absolute', 1, 1000],
            ['ram_error', 'error', 'absolute', 1, 1000]
        ]
    },
    'io': {
        'options': [None, 'IO Anomaly Scores', 'value', 'io', 'anomalies.io', 'line'],
        'lines': [
            ['io_expected', 'expected', 'absolute', 1, 1000],
            ['io_actual', 'io', 'absolute', 1, 1000],
            ['io_error', 'error', 'absolute', 1, 1000]
        ]
    },
    'net': {
        'options': [None, 'Network Anomaly Scores', 'value', 'net', 'anomalies.net', 'line'],
        'lines': [
            ['net_expected', 'expected', 'absolute', 1, 1000],
            ['net_actual', 'net', 'absolute', 1, 1000],
            ['net_error', 'error', 'absolute', 1, 1000]
        ]
    },
    'ip': {
        'options': [None, 'Network IP Anomaly Scores', 'value', 'ip', 'anomalies.ip', 'line'],
        'lines': [
            ['ip_expected', 'expected', 'absolute', 1, 1000],
            ['ip_actual', 'ip', 'absolute', 1, 1000],
            ['ip_error', 'error', 'absolute', 1, 1000]
        ]
    },
    'ipv6': {
        'options': [None, 'Network IPv6 Anomaly Scores', 'value', 'ipv6', 'anomalies.ipv6', 'line'],
        'lines': [
            ['ipv6_expected', 'expected', 'absolute', 1, 1000],
            ['ipv6_actual', 'ipv6', 'absolute', 1, 1000],
            ['ipv6_error', 'error', 'absolute', 1, 1000]
        ]
    },
    'processes': {
        'options': [None, 'Processes Anomaly Scores', 'value', 'processes', 'anomalies.processes', 'line'],
        'lines': [
            ['processes_expected', 'expected', 'absolute', 1, 1000],
            ['processes_actual', 'processes', 'absolute', 1, 1000],
            ['processes_error', 'error', 'absolute', 1, 1000]
        ]
    },
    'intr': {
        'options': [None, 'Interrupts Anomaly Scores', 'value', 'intr', 'anomalies.intr', 'line'],
        'lines': [
            ['intr_expected', 'expected', 'absolute', 1, 1000],
            ['intr_actual', 'processes', 'absolute', 1, 1000],
            ['intr_error', 'error', 'absolute', 1, 1000]
        ]
    },
    'scores': {
        'options': [None, 'All Anomaly Scores', 'value', 'scores', 'anomalies.scores', 'line'],
        'lines': [
            ['cpu_score', 'cpu', 'absolute', 1, 1000],
            ['load_score', 'load', 'absolute', 1, 1000],
            ['ram_score', 'ram', 'absolute', 1, 1000],
            ['io_score', 'io', 'absolute', 1, 1000],
            ['net_score', 'net', 'absolute', 1, 1000],
            ['ip_score', 'ip', 'absolute', 1, 1000],
            ['ipv6_score', 'ipv6', 'absolute', 1, 1000],
            ['processes_score', 'processes', 'absolute', 1, 1000],
            ['intr_score', 'intr', 'absolute', 1, 1000],
        ]
    },
    'anomalies': {
        'options': [None, 'All Anomaly Events', 'is_anomaly', 'anomalies', 'anomalies.anomalies', 'stacked'],
        'lines': [
            ['cpu_anomaly', 'cpu', 'absolute', 1, 1],
            ['load_anomaly', 'load', 'absolute', 1, 1],
            ['ram_anomaly', 'ram', 'absolute', 1, 1],
            ['io_anomaly', 'io', 'absolute', 1, 1],
            ['net_anomaly', 'net', 'absolute', 1, 1],
            ['ip_anomaly', 'ip', 'absolute', 1, 1],
            ['ipv6_anomaly', 'ipv6', 'absolute', 1, 1],
            ['processes_anomaly', 'processes', 'absolute', 1, 1],
            ['intr_anomaly', 'intr', 'absolute', 1, 1],
        ]
    },
}

HOST_PORT = '127.0.0.1:19999'
N = 20

def get_raw_data(host=None):

    if host is None:
        host = HOST_PORT

    data = dict()

    for chart in list(set(CHARTS.keys()) - set(['scores', 'anomalies'])):

        # get data
        url = f'http://{host}/api/v1/data?chart=system.{chart}&after=-{N}&format=json'
        response = requests.get(url)
        response_json = response.json()
        raw_data = response_json['data']
        df = pd.DataFrame(raw_data, columns=response_json['labels'])
        df = df.set_index('time').dropna()

        # create data for lines
        actual = np.mean(abs(df.values[0]))
        expected = np.mean(abs(df.values[1:]))
        error_abs = abs(actual - expected)
        error_pct = error_abs / expected
        anomalies = 1.0 if error_pct >= 0.8 else 0.0

        # add data
        data[f'{chart}_expected'] = expected * 1000
        data[f'{chart}_actual'] = actual * 1000
        data[f'{chart}_error'] = error_abs * 1000
        data[f'{chart}_score'] = error_pct * 1000
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

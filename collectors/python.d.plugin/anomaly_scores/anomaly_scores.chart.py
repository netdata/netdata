# -*- coding: utf-8 -*-
# Description: anomaly_scores netdata python.d module
# Author: andrewm4894
# SPDX-License-Identifier: GPL-3.0-or-later

from random import SystemRandom

from bases.FrameworkServices.SimpleService import SimpleService

priority = 90000

ORDER = [
    'cpu',
    'load',
    'disk',
    'network',
    'all'
]

CHARTS = {
    'cpu': {
        'options': [None, 'CPU Anomaly Scores', 'value', 'cpu', 'anomaly_scores.cpu', 'line'],
        'lines': [
            'expected', 'actual', 'error'
        ]
    },
    'load': {
        'options': [None, 'Load Anomaly Scores', 'value', 'load', 'anomaly_scores.load', 'line'],
        'lines': [
            'expected', 'actual', 'error'
        ]
    },
    'disk': {
        'options': [None, 'Disk Anomaly Scores', 'value', 'disk', 'anomaly_scores.disk', 'line'],
        'lines': [
            'expected', 'actual', 'error'
        ]
    },
    'network': {
        'options': [None, 'Network Anomaly Scores', 'value', 'network', 'anomaly_scores.network', 'line'],
        'lines': [
            'expected', 'actual', 'error'
        ]
    },
    'all': {
        'options': [None, 'All Anomaly Scores', 'value', 'all', 'anomaly_scores.all', 'line'],
        'lines': [
            'cpu', 'load', 'disk', 'network'
        ]
    },
}


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
        data = dict()

        for chart in ['cpu', 'load', 'disk', 'network']:

            dimension_id = 'expected'
            expected = self.random.randint(0, 50)
            if dimension_id not in self.charts[chart]:
                self.charts[chart].add_dimension([dimension_id])
            data[dimension_id] = expected

            dimension_id = 'actual'
            actual = expected + self.random.randint(-10, 10)
            if dimension_id not in self.charts[chart]:
                self.charts[chart].add_dimension([dimension_id])
            data[dimension_id] = actual

            dimension_id = 'error'
            error = actual - expected
            if dimension_id not in self.charts[chart]:
                self.charts[chart].add_dimension([dimension_id])
            data[dimension_id] = error

            if chart not in self.charts['all']:
                self.charts['all'].add_dimension([chart])
            data[chart] = error

        return data


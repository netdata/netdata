# -*- coding: utf-8 -*-
# Description: anomalies netdata python.d module
# Author: andrewm4894
# SPDX-License-Identifier: GPL-3.0-or-later

from random import SystemRandom, randint
import numpy as np

from bases.FrameworkServices.SimpleService import SimpleService

priority = 90000

ORDER = [
    'cpu',
    'load',
    'disk',
    'network',
    'scores',
    'anomalies',
]

CHARTS = {
    'cpu': {
        'options': [None, 'CPU Anomaly Scores', 'value', 'cpu', 'anomalies.cpu', 'line'],
        'lines': [
            'expected', 'actual', 'error'
        ]
    },
    'load': {
        'options': [None, 'Load Anomaly Scores', 'value', 'load', 'anomalies.load', 'line'],
        'lines': [
            'expected', 'actual', 'error'
        ]
    },
    'disk': {
        'options': [None, 'Disk Anomaly Scores', 'value', 'disk', 'anomalies.disk', 'line'],
        'lines': [
            'expected', 'actual', 'error'
        ]
    },
    'network': {
        'options': [None, 'Network Anomaly Scores', 'value', 'network', 'anomalies.network', 'line'],
        'lines': [
            'expected', 'actual', 'error'
        ]
    },
    'scores': {
        'options': [None, 'All Anomaly Scores', 'value', 'scores', 'anomalies.scores', 'line'],
        'lines': [
            'cpu_score', 'load_score', 'disk_score', 'network_score'
        ]
    },
    'anomalies': {
        'options': [None, 'All Anomaly Events', 'is_anomaly', 'anomalies', 'anomalies.anomalies', 'line'],
        'lines': [
            'cpu_anomaly', 'load_anomaly', 'disk_anomaly', 'network_anomaly'
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

    def add_dimension(self, chart, dimension_id):
        if dimension_id not in self.charts[chart]:
            self.charts[chart].add_dimension([dimension_id])

    def get_data(self):

        def add_dimension(self, chart, dimension_id):
            if dimension_id not in self.charts[chart]:
                self.charts[chart].add_dimension([dimension_id])

        data = dict()

        for chart in ['cpu', 'load', 'disk', 'network']:

            dimension_id = 'expected'
            expected = np.random.randint(0, 50)
            self.add_dimension(chart, dimension_id)
            data[dimension_id] = expected

            dimension_id = 'actual'
            actual = expected + np.random.randint(-10, 10)
            self.add_dimension(chart, dimension_id)
            data[dimension_id] = actual

            dimension_id = 'error'
            error = abs(actual - expected)
            self.add_dimension(chart, dimension_id)
            data[dimension_id] = error

            chart_score = f'{chart}_score'
            self.add_dimension(chart, dimension_id)
            data[chart_score] = error

            anomalies = 1 if error >= 9 else 0
            chart_anomalies = f'{chart}_anomalies'
            self.add_dimension('anomalies', chart_anomalies)
            data[chart_anomalies] = anomalies

        return data


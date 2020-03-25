# -*- coding: utf-8 -*-
# Description: anomaly_scores netdata python.d module
# Author: andrewm4894
# SPDX-License-Identifier: GPL-3.0-or-later

from random import SystemRandom

from bases.FrameworkServices.SimpleService import SimpleService

priority = 90000

ORDER = [
    'cpu',
]

CHARTS = {
    'cpu': {
        'options': [None, 'CPU Anomaly Scores', 'value', 'cpu', 'anomaly_scores.cpu', 'line'],
        'lines': [
            'expected', 'actual', 'error'
        ]
    }
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

        dimension_id = 'expected'
        expected = self.random.randint(0, 100)
        if dimension_id not in self.charts['cpu']:
            self.charts['cpu'].add_dimension([dimension_id])
        data[dimension_id] = expected

        dimension_id = 'actual'
        actual = expected + self.random.randint(-10, 10)
        if dimension_id not in self.charts['cpu']:
            self.charts['cpu'].add_dimension([dimension_id])
        data[dimension_id] = actual

        dimension_id = 'error'
        error = actual - expected
        if dimension_id not in self.charts['cpu']:
            self.charts['cpu'].add_dimension([dimension_id])
        data[dimension_id] = error

        return data


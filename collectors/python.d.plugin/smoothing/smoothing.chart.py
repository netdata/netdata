# -*- coding: utf-8 -*-
# Description: example netdata python.d module
# Author: Put your name here (your github login)
# SPDX-License-Identifier: GPL-3.0-or-later

from random import SystemRandom

import requests
import pandas as pd
from bases.FrameworkServices.SimpleService import SimpleService

priority = 1

ORDER = [
    'smoothing',
]

CHARTS = {
    'smoothing': {
        'options': [None, 'smoothing', 'smoothing', 'smoothing', 'smoothing', 'line'],
        'lines': [
        ]
    }
}

CHARTS_IN_SCOPE = ['system.cpu']
N = 2


def get_allmetrics(host: str = None, charts: list = None) -> list:
    url = f'http://{host}/api/v1/allmetrics?format=json'
    response = requests.get(url)
    raw_data = response.json()
    data = []
    for k in raw_data:
        if k in charts:
            time = raw_data[k]['last_updated']
            dimensions = raw_data[k]['dimensions']
            for dimension in dimensions:
                data.append([time, k, f"{k}.{dimensions[dimension]['name']}", dimensions[dimension]['value']])
    return data


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.random = SystemRandom()
        self.counter = 1
        self.data = []

    @staticmethod
    def check():
        return True

    def get_data(self):
        data = dict()

        for i in range(1, 4):
            dimension_id = ''.join(['random', str(i)])

            if dimension_id not in self.charts['random']:
                self.charts['random'].add_dimension([dimension_id])

            data[dimension_id] = self.random.randint(0, 100)

        return data

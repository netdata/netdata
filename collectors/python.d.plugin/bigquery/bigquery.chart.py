# -*- coding: utf-8 -*-
# Description: bigquery netdata python.d module
# Author: Andrew Maguire (andrewm4894)
# SPDX-License-Identifier: GPL-3.0-or-later

from random import SystemRandom
import pandas_gbq

from bases.FrameworkServices.SimpleService import SimpleService

priority = 90000

ORDER = [
    'random',
]

CHARTS = {
    'random': {
        'options': [None, 'A random number', 'random number', 'random', 'random', 'line'],
        'lines': [
            ['random1']
        ]
    }
}


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.random = SystemRandom()
        self.num_lines = self.configuration.get('num_lines', 4)
        self.lower = self.configuration.get('lower', 0)
        self.upper = self.configuration.get('upper', 100)
        self.sql = 'select rand() as random0, rand() as random1'

    @staticmethod
    def check():
        return True

    def get_data(self):
        data = dict()

        df = pandas_gbq.read_gbq(self.sql)
        data = df.to_dict('records')

        for dimension_id in df.columns:
            if dimension_id not in self.charts['random']:
                self.charts['random'].add_dimension([dimension_id])

        return data

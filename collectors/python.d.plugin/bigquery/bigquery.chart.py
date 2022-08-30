# -*- coding: utf-8 -*-
# Description: bigquery netdata python.d module
# Author: Andrew Maguire (andrewm4894)
# SPDX-License-Identifier: GPL-3.0-or-later

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

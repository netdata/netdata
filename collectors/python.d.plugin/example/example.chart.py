# -*- coding: utf-8 -*-
# Description: example netdata python.d module
# Author: Put your name here (your github login)
# SPDX-License-Identifier: GPL-3.0-or-later

from random import SystemRandom

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

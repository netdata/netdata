# -*- coding: utf-8 -*-
# Description: example netdata python.d module
# Author: Pawel Krupa (paulfantom)

import random

from bases.FrameworkServices.SimpleService import SimpleService

# default module values
# update_every = 4
priority = 90000
retries = 60

ORDER = ['random']
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
        super(self.__class__, self).__init__(configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.data = dict()

    @staticmethod
    def check():
        return True

    def get_data(self):
        dimension = ''.join(['random', str(random.randint(1, 3))])

        if dimension not in self.charts['random']:
            self.charts['random'].add_dimension([dimension])

        self.data[dimension] = random.randint(0, 100)

        return self.data


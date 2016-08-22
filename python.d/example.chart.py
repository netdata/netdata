# -*- coding: utf-8 -*-
# Description: example netdata python.d module
# Author: Pawel Krupa (paulfantom)

import os
import random
from base import SimpleService

NAME = os.path.basename(__file__).replace(".chart.py", "")

# default module values
# update_every = 4
priority = 90000
retries = 60


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        super(self.__class__,self).__init__(configuration=configuration, name=name)

    def check(self):
        return True
    
    def create(self):
        self.chart("example.python_random", '', 'A random number', 'random number',
                   'random', 'random', 'line', self.priority, self.update_every)
        self.dimension('random1')
        self.commit()
        return True
    
    def update(self, interval):
        self.begin("example.python_random", interval)
        self.set("random1", random.randint(0, 100))
        self.end()
        self.commit()
        return True

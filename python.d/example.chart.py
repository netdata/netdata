# -*- coding: utf-8 -*-
# Description: example netdata python.d plugin
# Author: Pawel Krupa (paulfantom)

import random
from base import BaseService

NAME = "example.chart.py"
# default module values
update_every = 3
priority = 90000
retries = 7


class Service(BaseService):
    def __init__(self, configuration=None, name=None):
        super(self.__class__,self).__init__(configuration=configuration, name=name)

    def check(self):
        return True
    
    def create(self):
        print("CHART example.python_random '' 'A random number' 'random number' random random line " +
              str(self.priority) +
              " " +
              str(self.update_every))
        print("DIMENSION random1 '' absolute 1 1")
        return True
    
    def update(self, interval):
        print("BEGIN example.python_random "+str(interval))
        print("SET random1 = "+str(random.randint(0,100)))
        print("END")
        return True

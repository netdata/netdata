# -*- coding: utf-8 -*-
# Description: example netdata python.d plugin
# Author: Pawel Krupa (paulfantom)

import os
import random
from base import BaseService

NAME = os.path.basename(__file__).replace(".chart.py", "")

# default module values
update_every = 4
priority = 90000
retries = 7


class Service(BaseService):
    def __init__(self, configuration=None, name=None):
        super(self.__class__,self).__init__(configuration=configuration, name=name)

    def check(self):
        return True
    
    def create(self):
        chart = "CHART example.python_random '' 'A random number' 'random number' random random line " + \
                str(self.priority) + " " + \
                str(self.update_every) + "\n" + \
                "DIMENSION random1 '' absolute 1 1"
        print(chart)
        return True
    
    def update(self, interval):
        chart = "BEGIN example.python_random "+str(interval)+"\n"
        chart += "SET random1 = "+str(random.randint(0,100))+"\n"
        print(chart + "END")
        return True

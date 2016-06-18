# Description: example netdata python.d plugin
# Author: Pawel Krupa (paulfantom)

NAME = "example.chart.py"
import sys
import random
from base import BaseService

# default module values
update_every = 3
priority = 90000
retries = 7


class Service(BaseService):
    def __init__(self,configuration=None,name=None):
        super().__init__(configuration=configuration)

    def check(self):
        return True
    
    def create(self):
        print("CHART example.python_random '' 'A random number' 'random number' random random line "+str(priority)+" 1")
        print("DIMENSION random1 '' absolute 1 1")
        return True
    
    def update(self,interval):
        print("BEGIN example.python_random "+str(interval))
        print("SET random1 = "+str(random.randint(0,100)))
        print("END")
        return True

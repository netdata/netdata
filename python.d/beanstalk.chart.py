# -*- coding: utf-8 -*-
# Description: Beanstalk monitor for netdata
# Author: Ed Wade (edwade3)

import beanstalkc
from base import SimpleService

NAME = "beanstalk.chart.py"

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
        self.chart("beanstalk.python_beanstalk", '', 'Beanstalk jobs', 'beanstalk',
                   '', 'random', 'line', self.priority, self.update_every)

        beanstalk = beanstalkc.Connection(host=self.host, self.port)

        tubes = self.beanstalk.tubes()
        for tube in tubes:
            stats = self.beanstalk.stats_tube(tube)
            for stat, value in stats.iteritems():
                self.dimension(tube + "_" + stat)
        self.commit()
        return True
    
    def update(self, interval):
        self.begin("beanstalk.python_beanstalk", interval)

        beanstalk = beanstalkc.Connection(host=self.host, self.port)

        tubes = self.beanstalk.tubes()
        for tube in tubes:
            print tube
            stats = self.beanstalk.stats_tube(tube)
            for stat, value in stats.iteritems():
                self.set(tube + "_" + stat, value)

        self.end()
        self.commit()
        return True

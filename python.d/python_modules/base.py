# Description: base for netdata python.d plugins
# Author: Pawel Krupa (paulfantom)

from time import time
import sys


class BaseService(object):
    def __init__(self, configuration=None, name=None):
        if configuration is None:
            self.error("BaseService: no configuration parameters supplied. Cannot create Service.")
            raise RuntimeError
        else:
            self._extract_base_config(configuration)
            self.timetable = {}
            self.create_timetable()
            self.execution_name = ""

    def _extract_base_config(self, config):
        self.update_every = int(config['update_every'])
        self.priority = int(config['priority'])
        self.retries = int(config['retries'])
        self.retries_left = self.retries

    def create_timetable(self, freq=None):
        if freq is None:
            freq = self.update_every
        now = time()
        self.timetable = {'last': now,
                          'next': now - (now % freq) + freq,
                          'freq': freq}

    @staticmethod
    def error(msg, exception=""):
        if exception != "":
            exception = " " + str(exception).replace("\n"," ")
        sys.stderr.write(str(msg)+exception+"\n")
        sys.stderr.flush()

    def check(self):
        self.error("Service " + str(self.__name__) + "doesn't implement check() function")
        return False

    def create(self):
        self.error("Service " + str(self.__name__) + "doesn't implement create() function?")
        return False

    def update(self):
        self.error("Service " + str(self.__name__) + "doesn't implement update() function")
        return False

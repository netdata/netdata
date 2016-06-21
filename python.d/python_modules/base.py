# Description: base for netdata python.d plugins
# Author: Pawel Krupa (paulfantom)

from time import time
import sys


class BaseService(object):
    """
    Prototype of Service class.
    Implemented basic functionality to run jobs by `python.d.plugin`
    """
    def __init__(self, configuration=None, name=None):
        """
        This needs to be initialized in child classes
        :param configuration: dict
        :param name: str
        """
        self.name = name
        if configuration is None:
            self.error("BaseService: no configuration parameters supplied. Cannot create Service.")
            raise RuntimeError
        else:
            self._extract_base_config(configuration)
            self.timetable = {}
            self.create_timetable()
            self.chart_name = ""

    def _extract_base_config(self, config):
        """
        Get basic parameters to run service
        Minimum config:
            config = {'update_every':1,
                      'priority':100000,
                      'retries':0}
        :param config: dict
        """
        self.update_every = int(config['update_every'])
        self.priority = int(config['priority'])
        self.retries = int(config['retries'])
        self.retries_left = self.retries

    def create_timetable(self, freq=None):
        """
        Create service timetable.
        `freq` is optional
        Example:
            timetable = {'last': 1466370091.3767564,
                         'next': 1466370092,
                         'freq': 1}
        :param freq: int
        """
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
        """
        check() prototype
        :return: boolean
        """
        self.error("Service " + str(self.__name__) + "doesn't implement check() function")
        return False

    def create(self):
        """
        create() prototype
        :return: boolean
        """
        self.error("Service " + str(self.__name__) + "doesn't implement create() function?")
        return False

    def update(self):
        """
        update() prototype
        :return: boolean
        """
        self.error("Service " + str(self.__name__) + "doesn't implement update() function")
        return False

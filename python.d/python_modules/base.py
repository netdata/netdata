# -*- coding: utf-8 -*-
# Description: prototypes for netdata python.d modules
# Author: Pawel Krupa (paulfantom)

from time import time
import sys
try:
    from urllib.request import urlopen
except ImportError:
    from urllib2 import urlopen


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
        self.update_every = int(config.pop('update_every'))
        self.priority = int(config.pop('priority'))
        self.retries = int(config.pop('retries'))
        self.retries_left = self.retries
        self.configuration = config

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
            exception = " " + str(exception).replace("\n", " ")
        sys.stderr.write(str(msg)+exception+"\n")
        sys.stderr.flush()

    def check(self):
        """
        check() prototype
        :return: boolean
        """
        self.error("Service " + str(self.__module__) + "doesn't implement check() function")
        return False

    def create(self):
        """
        create() prototype
        :return: boolean
        """
        self.error("Service " + str(self.__module__) + "doesn't implement create() function?")
        return False

    def update(self, interval):
        """
        update() prototype
        :param interval: int
        :return: boolean
        """
        self.error("Service " + str(self.__module__) + "doesn't implement update() function")
        return False


class UrlService(BaseService):
    def __init__(self, configuration=None, name=None):
        self.charts = {}
        # charts definitions in format:
        # charts = {
        #    'chart_name_in_netdata': {
        #        'options': "parameters defining chart (passed to CHART statement)",
        #        'lines': [
        #           { 'name': 'dimension_name',
        #             'options': 'dimension parameters (passed to DIMENSION statement)"
        #           }
        #        ]}
        #    }
        self.order = []
        self.definitions = {}
        # definitions are created dynamically in create() method based on 'charts' dictionary. format:
        # definitions = {
        #     'chart_name_in_netdata' : [ charts['chart_name_in_netdata']['lines']['name'] ]
        # }
        self.url = ""
        BaseService.__init__(self, configuration=configuration, name=name)

    def _get_data(self):
        """
        Get raw data from http request
        :return: str
        """
        raw = None
        try:
            f = urlopen(self.url, timeout=self.update_every)
            raw = f.read().decode('utf-8')
        except Exception as e:
            self.error(self.__module__, str(e))
        finally:
            try:
                f.close()
            except:
                pass
        return raw

    def _formatted_data(self):
        """
        Format data received from http request
        :return: dict
        """
        return {}

    def check(self):
        """
        Format configuration data and try to connect to server
        :return: boolean
        """
        if self.name is None:
            self.name = 'local'
        else:
            self.name = str(self.name)
        try:
            self.url = str(self.configuration['url'])
        except (KeyError, TypeError):
            pass

        if self._formatted_data() is not None:
            return True
        else:
            return False

    def create(self):
        """
        Create charts
        :return: boolean
        """
        for name in self.order:
            if name not in self.charts:
                continue
            self.definitions[name] = []
            for line in self.charts[name]['lines']:
                self.definitions[name].append(line['name'])

        idx = 0
        data = self._formatted_data()
        if data is None:
            return False
        for name in self.order:
            header = "CHART " + \
                     self.__module__ + "_" + \
                     self.name + "." + \
                     name + " " + \
                     self.charts[name]['options'] + " " + \
                     str(self.priority + idx) + " " + \
                     str(self.update_every)
            content = ""
            # check if server has this datapoint
            for line in self.charts[name]['lines']:
                if line['name'] in data:
                    content += "DIMENSION " + line['name'] + " " + line['options'] + "\n"

            if len(content) > 0:
                print(header)
                print(content)
                idx += 1

        if idx == 0:
            return False
        return True

    def update(self, interval):
        """
        Update charts
        :param interval: int
        :return: boolean
        """
        data = self._formatted_data()
        if data is None:
            return False

        for chart, dimensions in self.definitions.items():
            header = "BEGIN " + self.__module__ + "_" + str(self.name) + "." + chart + " " + str(interval)
            c = ""
            for dim in dimensions:
                try:
                    c += "\nSET " + dim + " = " + str(data[dim])
                except KeyError:
                    pass
            if len(c) != 0:
                print(header + c)
                print("END")

        return True

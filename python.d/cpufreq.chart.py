# -*- coding: utf-8 -*-
# Description: cpufreq netdata python.d module
# Author: Pawel Krupa (paulfantom)

import os
from base import SimpleService

# default module values (can be overridden per job in `config`)
# update_every = 2

ORDER = ['cpufreq']

CHARTS = {
    'cpufreq': {
        'options': [None, 'CPU Clock', 'MHz', 'cpufreq', None, 'line'],
        'lines': [
            # lines are created dynamically in `check()` method
        ]}
}


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        prefix = os.getenv('NETDATA_HOST_PREFIX', "")
        if prefix.endswith('/'):
            prefix = prefix[:-1]
        self.sys_dir = prefix + "/sys/devices"
        self.filename = "scaling_cur_freq"
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self._orig_name = ""
        self.assignment = {}
        self.paths = []

    def _get_data(self):
        raw = {}
        for path in self.paths:
            with open(path, 'r') as f:
                raw[path] = f.read()
        data = {}
        for path in self.paths:
            data[self.assignment[path]] = raw[path]
        return data

    def check(self):
        try:
            self.sys_dir = str(self.configuration['sys_dir'])
        except (KeyError, TypeError):
            self.error("No path specified. Using: '" + self.sys_dir + "'")

        self._orig_name = self.chart_name

        for dirpath, _, filenames in os.walk(self.sys_dir):
            if self.filename in filenames:
                self.paths.append(dirpath + "/" + self.filename)

        if len(self.paths) == 0:
            self.error("cannot find", self.filename)
            return False

        self.paths.sort()
        i = 0
        for path in self.paths:
            self.assignment[path] = "cpu" + str(i)
            i += 1

        for name in self.assignment:
            dim = self.assignment[name]
            self.definitions[ORDER[0]]['lines'].append([dim, dim, 'absolute', 1, 1000])

        return True

    def create(self):
        self.chart_name = "cpu"
        status = SimpleService.create(self)
        self.chart_name = self._orig_name
        return status

    def update(self, interval):
        self.chart_name = "cpu"
        status = SimpleService.update(self, interval=interval)
        self.chart_name = self._orig_name
        return status

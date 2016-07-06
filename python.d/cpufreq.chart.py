# -*- coding: utf-8 -*-
# Description: cpufreq netdata python.d plugin
# Author: Pawel Krupa (paulfantom)

from base import SysFileService

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


class Service(SysFileService):
    def __init__(self, configuration=None, name=None):
        SysFileService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self._orig_name = ""

    def check(self):
        self._orig_name = self.chart_name
        self.paths = self._find("scaling_cur_freq")

        if len(self.paths) == 0:
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
        status = SysFileService.create(self)
        self.chart_name = self._orig_name
        return status

    def update(self, interval):
        self.chart_name = "cpu"
        status = SysFileService.update(self, interval=interval)
        self.chart_name = self._orig_name
        return status

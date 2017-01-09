# -*- coding: utf-8 -*-
# Description: cpufreq netdata python.d module
# Author: Pawel Krupa (paulfantom) and Steven Noonan (tycho)

import glob
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
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self._orig_name = ""
        self.assignment = {}
        self.accurate = True

    def _get_data(self):
        data = {}
        if self.accurate:
            for name, path in self.assignment.items():
                total = 0
                for line in open(path, 'r'):
                    line = list(map(int, line.split()))
                    total += (line[0] * line[1]) / 100
                data[name] = total
        else:
            for name, path in self.assignment.items():
                data[name] = open(path, 'r').read()
        return data

    def check(self):
        try:
            self.sys_dir = str(self.configuration['sys_dir'])
        except (KeyError, TypeError):
            self.error("No path specified. Using: '" + self.sys_dir + "'")

        self._orig_name = self.chart_name

        for path in glob.glob(self.sys_dir + '/system/cpu/cpu*/cpufreq/stats/time_in_state'):
            if len(open(path, 'rb').read().rstrip()) == 0:
                self.alert("time_in_state is empty, broken cpufreq_stats data")
                self.assignment = {}
                break
            path_elem = path.split('/')
            cpu = path_elem[-4]
            self.assignment[cpu] = path

        if len(self.assignment) == 0:
            self.alert("trying less accurate scaling_cur_freq method")
            self.accurate = False

            for path in glob.glob(self.sys_dir + '/system/cpu/cpu*/cpufreq/scaling_cur_freq'):
                path_elem = path.split('/')
                cpu = path_elem[-3]
                self.assignment[cpu] = path

        if len(self.assignment) == 0:
            self.error("couldn't find a method to read cpufreq statistics")
            return False

        if self.accurate:
            algo = 'incremental'
        else:
            algo = 'absolute'

        for name in self.assignment.keys():
            self.definitions[ORDER[0]]['lines'].append([name, name, algo, 1, 1000])

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

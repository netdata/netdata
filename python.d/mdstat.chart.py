# -*- coding: utf-8 -*-
# Description: mdstat netdata python.d module
# Author: l2isbad
# SPDX-License-Identifier: GPL-3.0+

import re

from collections import defaultdict

from bases.FrameworkServices.SimpleService import SimpleService

priority = 60000
retries = 60
update_every = 1

ORDER = ['mdstat_health']
CHARTS = {
    'mdstat_health': {
        'options': [None, 'Faulty Devices In MD', 'failed disks', 'health', 'md.health', 'line'],
        'lines': list()
    }
}

OPERATIONS = ('check', 'resync', 'reshape', 'recovery', 'finish', 'speed')

RE_DISKS = re.compile(r' (?P<array>[a-zA-Z_0-9]+) : active .+\['
                      r'(?P<total_disks>[0-9]+)/'
                      r'(?P<inuse_disks>[0-9]+)\]')

RE_STATUS = re.compile(r' (?P<array>[a-zA-Z_0-9]+) : active .+ '
                       r'(?P<operation>[a-z]+) =[ ]{1,2}'
                       r'(?P<operation_status>[0-9.]+).+finish='
                       r'(?P<finish>([0-9.]+))min speed='
                       r'(?P<speed>[0-9]+)')


def md_charts(md):
    order = ['{0}_disks'.format(md.name),
             '{0}_operation'.format(md.name),
             '{0}_finish'.format(md.name),
             '{0}_speed'.format(md.name)
             ]

    charts = dict()
    charts[order[0]] = {
        'options': [None, 'Disks Stats', 'disks', md.name, 'md.disks', 'stacked'],
        'lines': [
            ['{0}_total_disks'.format(md.name), 'total', 'absolute'],
            ['{0}_inuse_disks'.format(md.name), 'inuse', 'absolute']
        ]
    }

    charts['_'.join([md.name, 'operation'])] = {
        'options': [None, 'Current Status', 'percent', md.name, 'md.status', 'line'],
        'lines': [
            ['{0}_resync'.format(md.name), 'resync', 'absolute', 1, 100],
            ['{0}_recovery'.format(md.name), 'recovery', 'absolute', 1, 100],
            ['{0}_reshape'.format(md.name), 'reshape', 'absolute', 1, 100],
            ['{0}_check'.format(md.name), 'check', 'absolute', 1, 100]
        ]
    }

    charts['_'.join([md.name, 'finish'])] = {
        'options': [None, 'Approximate Time Until Finish', 'seconds', md.name, 'md.rate', 'line'],
        'lines': [
            ['{0}_finish'.format(md.name), 'finish in', 'absolute', 1, 1000]
        ]
    }

    charts['_'.join([md.name, 'speed'])] = {
        'options': [None, 'Operation Speed', 'KB/s', md.name, 'md.rate', 'line'],
        'lines': [
            ['{0}_speed'.format(md.name), 'speed', 'absolute', 1, 1000]
        ]
    }

    return order, charts


class MD:
    def __init__(self, name, stats):
        self.name = name
        self.stats = stats

    def update_stats(self, stats):
        self.stats = stats

    def data(self):
        stats = dict(('_'.join([self.name, k]), v) for k, v in self.stats.items())
        stats['{0}_health'.format(self.name)] = int(self.stats['total_disks']) - int(self.stats['inuse_disks'])
        return stats


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.mds = dict()

    def check(self):
        arrays = find_arrays(self._get_raw_data())
        if not arrays:
            self.error('Failed to read data from /proc/mdstat or there is no active arrays')
            return None
        return True

    @staticmethod
    def _get_raw_data():
        """
        Read data from /proc/mdstat
        :return: str
        """
        try:
            with open('/proc/mdstat', 'rt') as proc_mdstat:
                return proc_mdstat.readlines() or None
        except (OSError, IOError):
            return None

    def get_data(self):
        """
        Parse data from _get_raw_data()
        :return: dict
        """
        arrays = find_arrays(self._get_raw_data())
        if not arrays:
            return None

        data = dict()
        for array, values in arrays.items():

            if array not in self.mds:
                md = MD(array, values)
                self.mds[md.name] = md
                self.create_new_array_charts(md)
            else:
                md = self.mds[array]
                md.update_stats(values)

            data.update(md.data())

        return data

    def create_new_array_charts(self, md):
        order, charts = md_charts(md)

        self.charts['mdstat_health'].add_dimension(['{0}_health'.format(md.name), md.name])
        for chart_name in order:
            params = [chart_name] + charts[chart_name]['options']
            dimensions = charts[chart_name]['lines']

            new_chart = self.charts.add_chart(params)
            for dimension in dimensions:
                new_chart.add_dimension(dimension)


def find_arrays(raw_data):
    if raw_data is None:
        return None
    data = defaultdict(str)
    counter = 1

    for row in (elem.strip() for elem in raw_data):
        if not row:
            counter += 1
            continue
        data[counter] = ' '.join([data[counter], row])

    arrays = dict()
    for value in data.values():
        match = RE_DISKS.search(value)
        if not match:
            continue

        match = match.groupdict()
        array = match.pop('array')
        arrays[array] = match
        for operation in OPERATIONS:
            arrays[array][operation] = 0

        match = RE_STATUS.search(value)
        if match:
            match = match.groupdict()
            if match['operation'] in OPERATIONS:
                arrays[array]['operation'] = match['operation']
                arrays[array][match['operation']] = float(match['operation_status']) * 100
                arrays[array]['finish'] = float(match['finish']) * 1000 * 60
                arrays[array]['speed'] = float(match['speed']) * 1000

    return arrays or None

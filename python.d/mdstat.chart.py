# -*- coding: utf-8 -*-
# Description: mdstat netdata python.d module
# Author: Ilya Mashchenko (l2isbad)
# SPDX-License-Identifier: GPL-3.0-or-later

import re

from collections import defaultdict

from bases.FrameworkServices.SimpleService import SimpleService

MDSTAT = '/proc/mdstat'
MISMATCH_CNT = '/sys/block/{0}/md/mismatch_cnt'

ORDER = ['mdstat_health']

CHARTS = {
    'mdstat_health': {
        'options': [None, 'Faulty Devices In MD', 'failed disks', 'health', 'md.health', 'line'],
        'lines': []
    }
}

RE_DISKS = re.compile(r' (?P<array>[a-zA-Z_0-9]+) : active .+\['
                      r'(?P<total_disks>[0-9]+)/'
                      r'(?P<inuse_disks>[0-9]+)\]')

RE_STATUS = re.compile(r' (?P<array>[a-zA-Z_0-9]+) : active .+ '
                       r'(?P<operation>[a-z]+) =[ ]{1,2}'
                       r'(?P<operation_status>[0-9.]+).+finish='
                       r'(?P<finish_in>([0-9.]+))min speed='
                       r'(?P<speed>[0-9]+)')


def md_charts(name):
    order = [
        '{0}_disks'.format(name),
        '{0}_operation'.format(name),
        '{0}_mismatch_cnt'.format(name),
        '{0}_finish'.format(name),
        '{0}_speed'.format(name)
    ]

    charts = dict()
    charts[order[0]] = {
        'options': [None, 'Disks Stats', 'disks', name, 'md.disks', 'stacked'],
        'lines': [
            ['{0}_total_disks'.format(name), 'total', 'absolute'],
            ['{0}_inuse_disks'.format(name), 'inuse', 'absolute']
        ]
    }

    charts[order[1]] = {
        'options': [None, 'Current Status', 'percent', name, 'md.status', 'line'],
        'lines': [
            ['{0}_resync'.format(name), 'resync', 'absolute', 1, 100],
            ['{0}_recovery'.format(name), 'recovery', 'absolute', 1, 100],
            ['{0}_reshape'.format(name), 'reshape', 'absolute', 1, 100],
            ['{0}_check'.format(name), 'check', 'absolute', 1, 100],
        ]
    }

    charts[order[2]] = {
        'options': [None, 'Mismatch Count', 'unsynchronized blocks', name, 'md.mismatch_cnt', 'line'],
        'lines': [
            ['{0}_mismatch_cnt'.format(name), 'count', 'absolute']
        ]
    }

    charts[order[3]] = {
        'options': [None, 'Approximate Time Until Finish', 'seconds', name, 'md.rate', 'line'],
        'lines': [
            ['{0}_finish_in'.format(name), 'finish in', 'absolute', 1, 1000]
        ]
    }

    charts[order[4]] = {
        'options': [None, 'Operation Speed', 'KB/s', name, 'md.rate', 'line'],
        'lines': [
            ['{0}_speed'.format(name), 'speed', 'absolute', 1, 1000]
        ]
    }

    return order, charts


class MD:
    def __init__(self, raw_data):
        self.name = raw_data['array']
        self.d = raw_data

    def data(self):
        rv = {
            'total_disks': self.d['total_disks'],
            'inuse_disks': self.d['inuse_disks'],
            'health': int(self.d['total_disks']) - int(self.d['inuse_disks']),
            'resync': 0,
            'recovery': 0,
            'reshape': 0,
            'check': 0,
            'finish_in': 0,
            'speed': 0,
        }

        v = read_lines(MISMATCH_CNT.format(self.name))
        if v:
            rv['mismatch_cnt'] = v

        if self.d.get('operation'):
            rv[self.d['operation']] = float(self.d['operation_status']) * 100
            rv['finish_in'] = float(self.d['finish_in']) * 1000 * 60
            rv['speed'] = float(self.d['speed']) * 1000

        return dict(('{0}_{1}'.format(self.name, k), v) for k, v in rv.items())


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.mds = list()

    @staticmethod
    def get_mds():
        raw = read_lines(MDSTAT)

        if not raw:
            return None

        return find_mds(raw)

    def get_data(self):
        """
        Parse data from _get_raw_data()
        :return: dict
        """
        mds = self.get_mds()

        if not mds:
            return None

        data = dict()
        for md in mds:
            if md.name not in self.mds:
                self.mds.append(md.name)
                self.add_new_md_charts(md.name)
            data.update(md.data())
        return data

    def check(self):
        if not self.get_mds():
            self.error('Failed to read data from {0} or there is no active arrays'.format(MDSTAT))
            return False
        return True

    def add_new_md_charts(self, name):
        order, charts = md_charts(name)

        self.charts['mdstat_health'].add_dimension(['{0}_health'.format(name), name])

        for chart_name in order:
            params = [chart_name] + charts[chart_name]['options']
            dims = charts[chart_name]['lines']

            chart = self.charts.add_chart(params)
            for dim in dims:
                chart.add_dimension(dim)


def find_mds(raw_data):
    data = defaultdict(str)
    counter = 1

    for row in (elem.strip() for elem in raw_data):
        if not row:
            counter += 1
            continue
        data[counter] = ' '.join([data[counter], row])

    mds = list()

    for v in data.values():
        m = RE_DISKS.search(v)

        if not m:
            continue

        d = m.groupdict()

        m = RE_STATUS.search(v)
        if m:
            d.update(m.groupdict())

        mds.append(MD(d))

    return sorted(mds, key=lambda md: md.name)


def read_lines(path):
    try:
        with open(path) as f:
            return f.readlines()
    except (IOError, OSError):
        return None

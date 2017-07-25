# -*- coding: utf-8 -*-
# Description: mdstat netdata python.d module
# Author: l2isbad

from re import compile as re_compile
from collections import defaultdict
from base import SimpleService

priority = 60000
retries = 60
update_every = 1

OPERATIONS = ('check', 'resync', 'reshape', 'recovery', 'finish', 'speed')


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.regex = dict(disks=re_compile(r' (?P<array>[a-zA-Z_0-9]+) : active .+\['
                                           r'(?P<total_disks>[0-9]+)/'
                                           r'(?P<inuse_disks>[0-9])\]'),
                          status=re_compile(r' (?P<array>[a-zA-Z_0-9]+) : active .+ '
                                            r'(?P<operation>[a-z]+) =[ ]{1,2}'
                                            r'(?P<operation_status>[0-9.]+).+finish='
                                            r'(?P<finish>([0-9.]+))min speed='
                                            r'(?P<speed>[0-9]+)'))

    def check(self):
        arrays = find_arrays(self._get_raw_data(), self.regex)
        if not arrays:
            self.error('Failed to read data from /proc/mdstat or there is no active arrays')
            return None

        self.order, self.definitions = create_charts(arrays.keys())
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

    def _get_data(self):
        """
        Parse data from _get_raw_data()
        :return: dict
        """
        raw_data = self._get_raw_data()
        arrays = find_arrays(raw_data, self.regex)
        if not arrays:
            return None

        to_netdata = dict()
        for array, values in arrays.items():
            for key, value in values.items():
                to_netdata['_'.join([array, key])] = value

        return to_netdata


def find_arrays(raw_data, regex):
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
        match = regex['disks'].search(value)
        if not match:
            continue

        match = match.groupdict()
        array = match.pop('array')
        arrays[array] = match
        arrays[array]['health'] = int(match['total_disks']) - int(match['inuse_disks'])
        for operation in OPERATIONS:
            arrays[array][operation] = 0

        match = regex['status'].search(value)
        if match:
            match = match.groupdict()
            if match['operation'] in OPERATIONS:
                arrays[array][match['operation']] = float(match['operation_status']) * 100
                arrays[array]['finish'] = float(match['finish']) * 100
                arrays[array]['speed'] = float(match['speed']) / 1000 * 100

    return arrays or None


def create_charts(arrays):
    order = ['mdstat_health']
    definitions = dict(mdstat_health={'options': [None, 'Faulty devices in MD', 'failed disks',
                                                  'health', 'md.health', 'line'],
                                      'lines': []})
    for md in arrays:
        order.append(md)
        order.append(md + '_status')
        order.append(md + '_rate')
        definitions['mdstat_health']['lines'].append([md + '_health', md, 'absolute'])
        definitions[md] = {'options': [None, '%s disks stats' % md, 'disks', md, 'md.disks', 'stacked'],
                           'lines': [[md + '_total_disks', 'total', 'absolute'],
                                     [md + '_inuse_disks', 'inuse', 'absolute']]}
        definitions[md + '_status'] = {'options': [None, '%s current status' % md,
                                                   'percent', md, 'md.status', 'line'],
                                       'lines': [[md + '_resync', 'resync', 'absolute', 1, 100],
                                                 [md + '_recovery', 'recovery', 'absolute', 1, 100],
                                                 [md + '_reshape', 'reshape', 'absolute', 1, 100],
                                                 [md + '_check', 'check', 'absolute', 1, 100]]}
        definitions[md + '_rate'] = {'options': [None, '%s operation status' % md,
                                                 'rate', md, 'md.rate', 'line'],
                                     'lines': [[md + '_finish', 'finish min', 'absolute', 1, 100],
                                               [md + '_speed', 'MB/s', 'absolute', -1, 100]]}
    return order, definitions

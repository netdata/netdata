# -*- coding: utf-8 -*-
# Description: bind rndc netdata python.d module
# Author: l2isbad
# SPDX-License-Identifier: GPL-3.0-or-later

import os

from collections import defaultdict
from subprocess import Popen

from bases.collection import find_binary
from bases.FrameworkServices.SimpleService import SimpleService

priority = 60000
retries = 60
update_every = 30

ORDER = ['name_server_statistics', 'incoming_queries', 'outgoing_queries', 'named_stats_size']

CHARTS = {
    'name_server_statistics': {
        'options': [None, 'Name Server Statistics', 'stats', 'name server statistics',
                    'bind_rndc.name_server_statistics', 'line'],
        'lines': [
            ['nms_requests', 'requests', 'incremental'],
            ['nms_rejected_queries', 'rejected_queries', 'incremental'],
            ['nms_success', 'success', 'incremental'],
            ['nms_failure', 'failure', 'incremental'],
            ['nms_responses', 'responses', 'incremental'],
            ['nms_duplicate', 'duplicate', 'incremental'],
            ['nms_recursion', 'recursion', 'incremental'],
            ['nms_nxrrset', 'nxrrset', 'incremental'],
            ['nms_nxdomain', 'nxdomain', 'incremental'],
            ['nms_non_auth_answer', 'non_auth_answer', 'incremental'],
            ['nms_auth_answer', 'auth_answer', 'incremental'],
            ['nms_dropped_queries', 'dropped_queries', 'incremental'],
        ]},
    'incoming_queries': {
        'options': [None, 'Incoming Queries', 'queries', 'incoming queries', 'bind_rndc.incoming_queries', 'line'],
        'lines': [
        ]},
    'outgoing_queries': {
        'options': [None, 'Outgoing Queries', 'queries', 'outgoing queries', 'bind_rndc.outgoing_queries', 'line'],
        'lines': [
        ]},
    'named_stats_size': {
        'options': [None, 'Named Stats File Size', 'MB', 'file size', 'bind_rndc.stats_size', 'line'],
        'lines': [
            ['stats_size', None, 'absolute', 1, 1 << 20]
        ]
    }
}

NMS = {
    'nms_requests': [
        'IPv4 requests received',
        'IPv6 requests received',
        'TCP requests received',
        'requests with EDNS(0) receive'
    ],
    'nms_responses': [
        'responses sent',
        'truncated responses sent',
        'responses with EDNS(0) sent',
        'requests with unsupported EDNS version received'
    ],
    'nms_failure': [
        'other query failures',
        'queries resulted in SERVFAIL'
    ],
    'nms_auth_answer': ['queries resulted in authoritative answer'],
    'nms_non_auth_answer': ['queries resulted in non authoritative answer'],
    'nms_nxrrset': ['queries resulted in nxrrset'],
    'nms_success': ['queries resulted in successful answer'],
    'nms_nxdomain': ['queries resulted in NXDOMAIN'],
    'nms_recursion': ['queries caused recursion'],
    'nms_duplicate': ['duplicate queries received'],
    'nms_rejected_queries': [
        'auth queries rejected',
        'recursive queries rejected'
    ],
    'nms_dropped_queries': ['queries dropped']
}

STATS = ['Name Server Statistics', 'Incoming Queries', 'Outgoing Queries']


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.named_stats_path = self.configuration.get('named_stats_path', '/var/log/bind/named.stats')
        self.rndc = find_binary('rndc')
        self.data = dict(nms_requests=0, nms_responses=0, nms_failure=0, nms_auth=0,
                         nms_non_auth=0, nms_nxrrset=0, nms_success=0, nms_nxdomain=0,
                         nms_recursion=0, nms_duplicate=0, nms_rejected_queries=0,
                         nms_dropped_queries=0)

    def check(self):
        if not self.rndc:
            self.error('Can\'t locate "rndc" binary or binary is not executable by netdata')
            return False

        if not (os.path.isfile(self.named_stats_path) and os.access(self.named_stats_path, os.R_OK)):
            self.error('Cannot access file %s' % self.named_stats_path)
            return False

        run_rndc = Popen([self.rndc, 'stats'], shell=False)
        run_rndc.wait()

        if not run_rndc.returncode:
            return True
        self.error('Not enough permissions to run "%s stats"' % self.rndc)
        return False

    def _get_raw_data(self):
        """
        Run 'rndc stats' and read last dump from named.stats
        :return: dict
        """
        result = dict()
        try:
            current_size = os.path.getsize(self.named_stats_path)
            run_rndc = Popen([self.rndc, 'stats'], shell=False)
            run_rndc.wait()

            if run_rndc.returncode:
                return None
            with open(self.named_stats_path) as named_stats:
                named_stats.seek(current_size)
                result['stats'] = named_stats.readlines()
                result['size'] = current_size
                return result
        except (OSError, IOError):
            return None

    def _get_data(self):
        """
        Parse data from _get_raw_data()
        :return: dict
        """

        raw_data = self._get_raw_data()

        if raw_data is None:
            return None
        parsed = dict()
        for stat in STATS:
            parsed[stat] = parse_stats(field=stat,
                                       named_stats=raw_data['stats'])

        self.data.update(nms_mapper(data=parsed['Name Server Statistics']))

        for elem in zip(['Incoming Queries', 'Outgoing Queries'], ['incoming_queries', 'outgoing_queries']):
            parsed_key, chart_name = elem[0], elem[1]
            for dimension_id, value in queries_mapper(data=parsed[parsed_key],
                                                      add=chart_name[:9]).items():

                if dimension_id not in self.data:
                    dimension = dimension_id.replace(chart_name[:9], '')
                    if dimension_id not in self.charts[chart_name]:
                        self.charts[chart_name].add_dimension([dimension_id, dimension, 'incremental'])

                self.data[dimension_id] = value

        self.data['stats_size'] = raw_data['size']
        return self.data


def parse_stats(field, named_stats):
    """
    :param field: str:
    :param named_stats: list:
    :return: dict

    Example:
    filed: 'Incoming Queries'
    names_stats (list of lines):
    ++ Incoming Requests ++
             1405660 QUERY
                   3 NOTIFY
    ++ Incoming Queries ++
             1214961 A
                  75 NS
                   2 CNAME
                2897 SOA
               35544 PTR
                  14 MX
                5822 TXT
              145974 AAAA
                 371 SRV
    ++ Outgoing Queries ++
    ...

    result:
    {'A', 1214961, 'NS': 75, 'CNAME': 2, 'SOA': 2897, ...}
    """
    data = dict()
    ns = iter(named_stats)
    for line in ns:
        if field not in line:
            continue
        while True:
            try:
                line = next(ns)
            except StopIteration:
                break
            if '++' not in line:
                if '[' in line:
                    continue
                v, k = line.strip().split(' ', 1)
                data[k] = int(v)
                continue
            break
        break
    return data


def nms_mapper(data):
    """
    :param data: dict
    :return: dict(defaultdict)
    """
    result = defaultdict(int)
    for k, v in NMS.items():
        for elem in v:
            result[k] += data.get(elem, 0)
    return result


def queries_mapper(data, add):
    """
    :param data: dict
    :param add: str
    :return: dict
    """
    return dict([(add + k, v) for k, v in data.items()])

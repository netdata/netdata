# -*- coding: utf-8 -*-
# Description: litespeed netdata python.d module
# Author: Ilya Maschenko (ilyam8)
# SPDX-License-Identifier: GPL-3.0-or-later

import glob
import os
import re
from collections import namedtuple

from bases.FrameworkServices.SimpleService import SimpleService

update_every = 10

# charts order (can be overridden if you want less charts, or different order)
ORDER = [
    'net_throughput_http',  # net throughput
    'net_throughput_https',  # net throughput
    'connections_http',  # connections
    'connections_https',  # connections
    'requests',  # requests
    'requests_processing',  # requests
    'pub_cache_hits',  # cache
    'private_cache_hits',  # cache
    'static_hits',  # static
]

CHARTS = {
    'net_throughput_http': {
        'options': [None, 'Network Throughput HTTP', 'kilobits/s', 'net throughput',
                    'litespeed.net_throughput', 'area'],
        'lines': [
            ['bps_in', 'in', 'absolute'],
            ['bps_out', 'out', 'absolute', -1]
        ]
    },
    'net_throughput_https': {
        'options': [None, 'Network Throughput HTTPS', 'kilobits/s', 'net throughput',
                    'litespeed.net_throughput', 'area'],
        'lines': [
            ['ssl_bps_in', 'in', 'absolute'],
            ['ssl_bps_out', 'out', 'absolute', -1]
        ]
    },
    'connections_http': {
        'options': [None, 'Connections HTTP', 'conns', 'connections', 'litespeed.connections', 'stacked'],
        'lines': [
            ['conn_free', 'free', 'absolute'],
            ['conn_used', 'used', 'absolute']
        ]
    },
    'connections_https': {
        'options': [None, 'Connections HTTPS', 'conns', 'connections', 'litespeed.connections', 'stacked'],
        'lines': [
            ['ssl_conn_free', 'free', 'absolute'],
            ['ssl_conn_used', 'used', 'absolute']
        ]
    },
    'requests': {
        'options': [None, 'Requests', 'requests/s', 'requests', 'litespeed.requests', 'line'],
        'lines': [
            ['requests', None, 'absolute', 1, 100]
        ]
    },
    'requests_processing': {
        'options': [None, 'Requests In Processing', 'requests', 'requests', 'litespeed.requests_processing', 'line'],
        'lines': [
            ['requests_processing', 'processing', 'absolute']
        ]
    },
    'pub_cache_hits': {
        'options': [None, 'Public Cache Hits', 'hits/s', 'cache', 'litespeed.cache', 'line'],
        'lines': [
            ['pub_cache_hits', 'hits', 'absolute', 1, 100]
        ]
    },
    'private_cache_hits': {
        'options': [None, 'Private Cache Hits', 'hits/s', 'cache', 'litespeed.cache', 'line'],
        'lines': [
            ['private_cache_hits', 'hits', 'absolute', 1, 100]
        ]
    },
    'static_hits': {
        'options': [None, 'Static Hits', 'hits/s', 'static', 'litespeed.static', 'line'],
        'lines': [
            ['static_hits', 'hits', 'absolute', 1, 100]
        ]
    }
}

t = namedtuple('T', ['key', 'id', 'mul'])

T = [
    t('BPS_IN', 'bps_in', 8),
    t('BPS_OUT', 'bps_out', 8),
    t('SSL_BPS_IN', 'ssl_bps_in', 8),
    t('SSL_BPS_OUT', 'ssl_bps_out', 8),
    t('REQ_PER_SEC', 'requests', 100),
    t('REQ_PROCESSING', 'requests_processing', 1),
    t('PUB_CACHE_HITS_PER_SEC', 'pub_cache_hits', 100),
    t('PRIVATE_CACHE_HITS_PER_SEC', 'private_cache_hits', 100),
    t('STATIC_HITS_PER_SEC', 'static_hits', 100),
    t('PLAINCONN', 'conn_used', 1),
    t('AVAILCONN', 'conn_free', 1),
    t('SSLCONN', 'ssl_conn_used', 1),
    t('AVAILSSL', 'ssl_conn_free', 1),
]

RE = re.compile(r'([A-Z_]+): ([0-9.]+)')

ZERO_DATA = {
    'bps_in': 0,
    'bps_out': 0,
    'ssl_bps_in': 0,
    'ssl_bps_out': 0,
    'requests': 0,
    'requests_processing': 0,
    'pub_cache_hits': 0,
    'private_cache_hits': 0,
    'static_hits': 0,
    'conn_used': 0,
    'conn_free': 0,
    'ssl_conn_used': 0,
    'ssl_conn_free': 0,
}


class Service(SimpleService):
    def __init__(self, configuration=None, name=None):
        SimpleService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.path = self.configuration.get('path', '/tmp/lshttpd/')
        self.files = list()

    def check(self):
        if not self.path:
            self.error('"path" not specified')
            return False

        fs = glob.glob(os.path.join(self.path, '.rtreport*'))

        if not fs:
            self.error('"{0}" has no "rtreport" files or dir is not readable'.format(self.path))
            return None

        self.debug('stats files:', fs)

        for f in fs:
            if not is_readable_file(f):
                self.error('{0} is not readable'.format(f))
                continue
            self.files.append(f)

        return bool(self.files)

    def get_data(self):
        """
        Format data received from http request
        :return: dict
        """
        data = dict(ZERO_DATA)

        for f in self.files:
            try:
                with open(f) as b:
                    lines = b.readlines()
            except (OSError, IOError) as err:
                self.error(err)
                return None
            else:
                parse_file(data, lines)

        return data


def parse_file(data, lines):
    for line in lines:
        if not line.startswith(('BPS_IN:', 'MAXCONN:', 'PLAINCONN:', 'REQ_RATE []:')):
            continue
        m = dict(RE.findall(line))
        for v in T:
            if v.key in m:
                data[v.id] += float(m[v.key]) * v.mul


def is_readable_file(v):
    return os.path.isfile(v) and os.access(v, os.R_OK)

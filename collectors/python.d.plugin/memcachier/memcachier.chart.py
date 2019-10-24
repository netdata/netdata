# -*- coding: utf-8 -*-
# Description: memcachier netdata python.d module
# Author: José Canal (joseliber)
# SPDX-License-Identifier: GPL-3.0-or-later

import json

from bases.FrameworkServices.UrlService import UrlService


ORDER = [
    'cache',
    'net',
    'connections',
    'items',
    'evicted',
    'expired',
    'get',
    'get_rate',
    'set_rate',
    'flush_rate',
    'delete_rate',
    'cas',
    'delete',
    'increment',
    'decrement',
    'touch',
    'touch_rate',
]

CHARTS = {
    'cache': {
        'options': [None, 'Cache Size', 'MiB', 'cache', 'memcachier.cache', 'stacked'],
        'lines': [
            ['avail', 'available', 'absolute', 1, 1 << 20],
            ['used', 'used', 'absolute', 1, 1 << 20]
        ]
    },
    'net': {
        'options': [None, 'Network', 'kilobits/s', 'network', 'memcachier.net', 'area'],
        'lines': [
            ['bytes_read', 'in', 'incremental', 8, 1000],
            ['bytes_written', 'out', 'incremental', -8, 1000],
        ]
    },
    'connections': {
        'options': [None, 'Connections', 'connections/s', 'connections', 'memcachier.connections', 'line'],
        'lines': [
            ['curr_connections', 'current', 'incremental'],
            ['total_connections', 'total', 'incremental']
        ]
    },
    'items': {
        'options': [None, 'Items', 'items', 'items', 'memcachier.items', 'line'],
        'lines': [
            ['curr_items', 'current', 'absolute'],
            ['total_items', 'total', 'absolute']
        ]
    },
    'evicted': {
        'options': [None, 'Items', 'items', 'items', 'memcachier.evicted', 'line'],
        'lines': [
            ['evictions', 'evicted', 'absolute']
        ]
    },
    'expired': {
        'options': [None, 'Items', 'items', 'items', 'memcachier.expired', 'line'],
        'lines': [
            ['expired', 'expired', 'absolute']
        ]
    },
    'get': {
        'options': [None, 'Requests', 'requests', 'get ops', 'memcachier.get', 'stacked'],
        'lines': [
            ['get_hits', 'hits', 'percent-of-absolute-row'],
            ['get_misses', 'misses', 'percent-of-absolute-row']
        ]
    },
    'get_rate': {
        'options': [None, 'Rate', 'requests/s', 'get ops', 'memcachier.get_rate', 'line'],
        'lines': [
            ['cmd_get', 'rate', 'incremental']
        ]
    },
    'set_rate': {
        'options': [None, 'Rate', 'requests/s', 'set ops', 'memcachier.set_rate', 'line'],
        'lines': [
            ['cmd_set', 'rate', 'incremental']
        ]
    },
    'flush_rate': {
        'options': [None, 'Rate', 'requests/s', 'flush ops', 'memcachier.flush_rate', 'line'],
        'lines': [
            ['cmd_flush', 'rate', 'incremental']
        ]
    },
    'delete_rate': {
        'options': [None, 'Rate', 'requests/s', 'delete ops', 'memcachier.delete_rate', 'line'],
        'lines': [
            ['cmd_delete', 'rate', 'incremental']
        ]
    },
    'delete': {
        'options': [None, 'Requests', 'requests', 'delete ops', 'memcachier.delete', 'stacked'],
        'lines': [
            ['delete_hits', 'hits', 'percent-of-absolute-row'],
            ['delete_misses', 'misses', 'percent-of-absolute-row'],
        ]
    },
    'cas': {
        'options': [None, 'Requests', 'requests', 'check and set ops', 'memcachier.cas', 'stacked'],
        'lines': [
            ['cas_hits', 'hits', 'percent-of-absolute-row'],
            ['cas_misses', 'misses', 'percent-of-absolute-row'],
            ['cas_badval', 'bad value', 'percent-of-absolute-row']
        ]
    },
    'increment': {
        'options': [None, 'Requests', 'requests', 'increment ops', 'memcachier.increment', 'stacked'],
        'lines': [
            ['incr_hits', 'hits', 'percent-of-absolute-row'],
            ['incr_misses', 'misses', 'percent-of-absolute-row']
        ]
    },
    'decrement': {
        'options': [None, 'Requests', 'requests', 'decrement ops', 'memcachier.decrement', 'stacked'],
        'lines': [
            ['decr_hits', 'hits', 'percent-of-absolute-row'],
            ['decr_misses', 'misses', 'percent-of-absolute-row']
        ]
    },
    'touch': {
        'options': [None, 'Requests', 'requests', 'touch ops', 'memcachier.touch', 'stacked'],
        'lines': [
            ['touch_hits', 'hits', 'percent-of-absolute-row'],
            ['touch_misses', 'misses', 'percent-of-absolute-row']
        ]
    },
    'touch_rate': {
        'options': [None, 'Rate', 'requests/s', 'touch ops', 'memcachier.touch_rate', 'line'],
        'lines': [
            ['cmd_touch', 'rate', 'incremental']
        ]
    }
}


class Service(UrlService):
    def __init__(self, configuration=None, name=None):
        UrlService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.url = 'https://analytics.memcachier.com/api/v1/login'
        self.user = self.configuration.get('user')
        self.password = self.configuration.get('pass')

    def _get_data(self):
        """
        Get data from API
        :return: dict
        """
        response = self._get_raw_data()
        if response is None:
            return None

        metrics = json.loads(response)

        # check if we have more than one server in the results
        if len(metrics) > 1:
            self.error('This plugin only works with one cache_server per job.')
            self.debug('Request result: {r}'.format(r=metrics))
            return None

        data = list(metrics.values())[0]

        # custom calculations
        try:
            data['avail'] = int(data['limit_maxbytes']) - int(data['bytes'])
            data['used'] = int(data['bytes'])
        except (KeyError, ValueError, TypeError):
            pass

        return data

    def check(self):
        """
        Create manager, get cache_id and update url
        :return: boolean
        """
        self._manager = self._build_manager()
        data = self._get_raw_data()

        if data is None:
            return False

        # we need to guarantee that only one cache_id is passed forward
        data = json.loads(data)
        if len(data) == 1 and type(data['cache_id']) == int:
            self.url = 'https://analytics.memcachier.com/api/v1/{id}/stats'.format(id=data['cache_id'])
            return True

        self.error('This plugin only works with one cache_id per job.')
        self.debug('Request result: {r}'.format(r=data))
        return False
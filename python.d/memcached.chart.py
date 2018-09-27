# -*- coding: utf-8 -*-
# Description: memcached netdata python.d module
# Author: Pawel Krupa (paulfantom)
# SPDX-License-Identifier: GPL-3.0+

from bases.FrameworkServices.SocketService import SocketService

# default module values (can be overridden per job in `config`)
#update_every = 2
priority = 60000
retries = 60

# default job configuration (overridden by python.d.plugin)
# config = {'local': {
#             'update_every': update_every,
#             'retries': retries,
#             'priority': priority,
#             'host': 'localhost',
#             'port': 11211,
#             'unix_socket': None
#          }}

ORDER = ['cache', 'net', 'connections', 'items', 'evicted_reclaimed',
         'get', 'get_rate', 'set_rate', 'cas', 'delete', 'increment', 'decrement', 'touch', 'touch_rate']

CHARTS = {
    'cache': {
        'options': [None, 'Cache Size', 'megabytes', 'cache', 'memcached.cache', 'stacked'],
        'lines': [
            ['avail', 'available', 'absolute', 1, 1048576],
            ['used', 'used', 'absolute', 1, 1048576]
        ]
    },
    'net': {
        'options': [None, 'Network', 'kilobits/s', 'network', 'memcached.net', 'area'],
        'lines': [
            ['bytes_read', 'in', 'incremental', 8, 1024],
            ['bytes_written', 'out', 'incremental', -8, 1024]
        ]
    },
    'connections': {
        'options': [None, 'Connections', 'connections/s', 'connections', 'memcached.connections', 'line'],
        'lines': [
            ['curr_connections', 'current', 'incremental'],
            ['rejected_connections', 'rejected', 'incremental'],
            ['total_connections', 'total', 'incremental']
        ]
    },
    'items': {
        'options': [None, 'Items', 'items', 'items', 'memcached.items', 'line'],
        'lines': [
            ['curr_items', 'current', 'absolute'],
            ['total_items', 'total', 'absolute']
        ]
    },
    'evicted_reclaimed': {
        'options': [None, 'Items', 'items', 'items', 'memcached.evicted_reclaimed', 'line'],
        'lines': [
            ['reclaimed', 'reclaimed', 'absolute'],
            ['evictions', 'evicted', 'absolute']
        ]
    },
    'get': {
        'options': [None, 'Requests', 'requests', 'get ops', 'memcached.get', 'stacked'],
        'lines': [
            ['get_hits', 'hits', 'percent-of-absolute-row'],
            ['get_misses', 'misses', 'percent-of-absolute-row']
        ]
    },
    'get_rate': {
        'options': [None, 'Rate', 'requests/s', 'get ops', 'memcached.get_rate', 'line'],
        'lines': [
            ['cmd_get', 'rate', 'incremental']
        ]
    },
    'set_rate': {
        'options': [None, 'Rate', 'requests/s', 'set ops', 'memcached.set_rate', 'line'],
        'lines': [
            ['cmd_set', 'rate', 'incremental']
        ]
    },
    'delete': {
        'options': [None, 'Requests', 'requests', 'delete ops', 'memcached.delete', 'stacked'],
        'lines': [
            ['delete_hits', 'hits', 'percent-of-absolute-row'],
            ['delete_misses', 'misses', 'percent-of-absolute-row'],
        ]
    },
    'cas': {
        'options': [None, 'Requests', 'requests', 'check and set ops', 'memcached.cas', 'stacked'],
        'lines': [
            ['cas_hits', 'hits', 'percent-of-absolute-row'],
            ['cas_misses', 'misses', 'percent-of-absolute-row'],
            ['cas_badval', 'bad value', 'percent-of-absolute-row']
        ]
    },
    'increment': {
        'options': [None, 'Requests', 'requests', 'increment ops', 'memcached.increment', 'stacked'],
        'lines': [
            ['incr_hits', 'hits', 'percent-of-absolute-row'],
            ['incr_misses', 'misses', 'percent-of-absolute-row']
        ]
    },
    'decrement': {
        'options': [None, 'Requests', 'requests', 'decrement ops', 'memcached.decrement', 'stacked'],
        'lines': [
            ['decr_hits', 'hits', 'percent-of-absolute-row'],
            ['decr_misses', 'misses', 'percent-of-absolute-row']
        ]
    },
    'touch': {
        'options': [None, 'Requests', 'requests', 'touch ops', 'memcached.touch', 'stacked'],
        'lines': [
            ['touch_hits', 'hits', 'percent-of-absolute-row'],
            ['touch_misses', 'misses', 'percent-of-absolute-row']
        ]
    },
    'touch_rate': {
        'options': [None, 'Rate', 'requests/s', 'touch ops', 'memcached.touch_rate', 'line'],
        'lines': [
            ['cmd_touch', 'rate', 'incremental']
        ]
    }
}


class Service(SocketService):
    def __init__(self, configuration=None, name=None):
        SocketService.__init__(self, configuration=configuration, name=name)
        self.request = 'stats\r\n'
        self.host = 'localhost'
        self.port = 11211
        self._keep_alive = True
        self.unix_socket = None
        self.order = ORDER
        self.definitions = CHARTS

    def _get_data(self):
        """
        Get data from socket
        :return: dict
        """
        response = self._get_raw_data()
        if response is None:
            # error has already been logged
            return None

        if response.startswith('ERROR'):
            self.error('received ERROR')
            return None

        try:
            parsed = response.split('\n')
        except AttributeError:
            self.error('response is invalid/empty')
            return None

        # split the response
        data = {}
        for line in parsed:
            if line.startswith('STAT'):
                try:
                    t = line[5:].split(' ')
                    data[t[0]] = t[1]
                except (IndexError, ValueError):
                    self.debug('invalid line received: ' + str(line))

        if not data:
            self.error("received data doesn't have any records")
            return None

        # custom calculations
        try:
            data['avail'] = int(data['limit_maxbytes']) - int(data['bytes'])
            data['used'] = int(data['bytes'])
        except (KeyError, ValueError, TypeError):
            pass

        return data

    def _check_raw_data(self, data):
        if data.endswith('END\r\n'):
            self.debug('received full response from memcached')
            return True

        self.debug('waiting more data from memcached')
        return False

    def check(self):
        """
        Parse configuration, check if memcached is available
        :return: boolean
        """
        self._parse_config()
        data = self._get_data()
        if data is None:
            return False
        return True

# -*- coding: utf-8 -*-
# Description: redis netdata python.d module
# Author: Pawel Krupa (paulfantom)
# Author: Ilya Mashchenko (l2isbad)
# SPDX-License-Identifier: GPL-3.0+

import re

from copy import deepcopy

from bases.FrameworkServices.SocketService import SocketService

REDIS_ORDER = [
    'operations',
    'hit_rate',
    'memory',
    'keys_redis',
    'eviction',
    'net',
    'connections',
    'clients',
    'slaves',
    'persistence',
    'bgsave_now',
    'bgsave_health',
    'uptime',
]

PIKA_ORDER = [
    'operations',
    'hit_rate',
    'memory',
    'keys_pika',
    'connections',
    'clients',
    'slaves',
    'uptime',
]


CHARTS = {
    'operations': {
        'options': [None, 'Operations', 'operations/s', 'operations', 'redis.operations', 'line'],
        'lines': [
            ['total_commands_processed', 'commands', 'incremental'],
            ['instantaneous_ops_per_sec', 'operations', 'absolute']
        ]
    },
    'hit_rate': {
        'options': [None, 'Hit rate', 'percent', 'hits', 'redis.hit_rate', 'line'],
        'lines': [
            ['hit_rate', 'rate', 'absolute']
        ]
    },
    'memory': {
        'options': [None, 'Memory utilization', 'kilobytes', 'memory', 'redis.memory', 'line'],
        'lines': [
            ['used_memory', 'total', 'absolute', 1, 1024],
            ['used_memory_lua', 'lua', 'absolute', 1, 1024]
        ]
    },
    'net': {
        'options': [None, 'Bandwidth', 'kilobits/s', 'network', 'redis.net', 'area'],
        'lines': [
            ['total_net_input_bytes', 'in', 'incremental', 8, 1024],
            ['total_net_output_bytes', 'out', 'incremental', -8, 1024]
        ]
    },
    'keys_redis': {
        'options': [None, 'Keys per Database', 'keys', 'keys', 'redis.keys', 'line'],
        'lines': []
    },
    'keys_pika': {
        'options': [None, 'Keys', 'keys', 'keys', 'redis.keys', 'line'],
        'lines': [
            ['kv_keys', 'kv', 'absolute'],
            ['hash_keys', 'hash', 'absolute'],
            ['list_keys', 'list', 'absolute'],
            ['zset_keys', 'zset', 'absolute'],
            ['set_keys', 'set', 'absolute']
        ]
    },
    'eviction': {
        'options': [None, 'Evicted Keys', 'keys', 'keys', 'redis.eviction', 'line'],
        'lines': [
            ['evicted_keys', 'evicted', 'absolute']
        ]
    },
    'connections': {
        'options': [None, 'Connections', 'connections/s', 'connections', 'redis.connections', 'line'],
        'lines': [
            ['total_connections_received', 'received', 'incremental', 1],
            ['rejected_connections', 'rejected', 'incremental', -1]
        ]
    },
    'clients': {
        'options': [None, 'Clients', 'clients', 'connections', 'redis.clients', 'line'],
        'lines': [
            ['connected_clients', 'connected', 'absolute', 1],
            ['blocked_clients', 'blocked', 'absolute', -1]
        ]
    },
    'slaves': {
        'options': [None, 'Slaves', 'slaves', 'replication', 'redis.slaves', 'line'],
        'lines': [
            ['connected_slaves', 'connected', 'absolute']
        ]
    },
    'persistence': {
        'options': [None, 'Persistence Changes Since Last Save', 'changes', 'persistence',
                    'redis.rdb_changes', 'line'],
        'lines': [
            ['rdb_changes_since_last_save', 'changes', 'absolute']
        ]
    },
    'bgsave_now': {
        'options': [None, 'Duration of the RDB Save Operation', 'seconds', 'persistence',
                    'redis.bgsave_now', 'absolute'],
        'lines': [
            ['rdb_bgsave_in_progress', 'rdb save', 'absolute']
        ]
    },
    'bgsave_health': {
        'options': [None, 'Status of the Last RDB Save Operation', 'status', 'persistence',
                    'redis.bgsave_health', 'line'],
        'lines': [
            ['rdb_last_bgsave_status', 'rdb save', 'absolute']
        ]
    },
    'uptime': {
        'options': [None, 'Uptime', 'seconds', 'uptime', 'redis.uptime', 'line'],
        'lines': [
            ['uptime_in_seconds', 'uptime', 'absolute']
        ]
    }
}


def copy_chart(name):
    return {name: deepcopy(CHARTS[name])}


RE = re.compile(r'\n([a-z_0-9 ]+):(?:keys=)?([^,\r]+)')


class Service(SocketService):
    def __init__(self, configuration=None, name=None):
        SocketService.__init__(self, configuration=configuration, name=name)
        self._keep_alive = True

        self.order = list()
        self.definitions = dict()

        self.host = self.configuration.get('host', 'localhost')
        self.port = self.configuration.get('port', 6379)
        self.unix_socket = self.configuration.get('socket')
        p = self.configuration.get('pass')

        self.auth_request = 'AUTH {0} \r\n'.format(p).encode() if p else None
        self.request = 'INFO\r\n'.encode()
        self.bgsave_time = 0

    def do_auth(self):
        resp = self._get_raw_data(request=self.auth_request)
        if not resp:
            return False
        if resp.strip() != '+OK':
            self.error('invalid password')
            return False
        return True

    def get_raw_and_parse(self):
        if self.auth_request and not self.do_auth():
            return None

        resp = self._get_raw_data()

        if not resp:
            return None

        parsed = RE.findall(resp)

        if not parsed:
            self.error('response is invalid/empty')
            return None

        return dict((k.replace(' ', '_'), v) for k, v in parsed)

    def get_data(self):
        """
        Get data from socket
        :return: dict
        """
        data = self.get_raw_and_parse()

        if not data:
            return None

        try:
            data['hit_rate'] = (
                    (int(data['keyspace_hits']) * 100) / (int(data['keyspace_hits']) + int(data['keyspace_misses']))
            )
        except (KeyError, ZeroDivisionError):
            data['hit_rate'] = 0

        if data.get('redis_version') and data.get('rdb_bgsave_in_progress'):
            self.get_data_redis_specific(data)

        return data

    def get_data_redis_specific(self, data):
        if data['rdb_bgsave_in_progress'] != '0':
            self.bgsave_time += self.update_every
        else:
            self.bgsave_time = 0

        data['rdb_last_bgsave_status'] = 0 if data['rdb_last_bgsave_status'] == 'ok' else 1
        data['rdb_bgsave_in_progress'] = self.bgsave_time

    def check(self):
        """
        Parse configuration, check if redis is available, and dynamically create chart lines data
        :return: boolean
        """
        data = self.get_raw_and_parse()

        if not data:
            return False

        self.order = PIKA_ORDER if data.get('pika_version') else REDIS_ORDER

        for n in self.order:
            self.definitions.update(copy_chart(n))

        if data.get('redis_version'):
            for k in data:
                if k.startswith('db'):
                    self.definitions['keys_redis']['lines'].append([k, None, 'absolute'])

        return True

    def _check_raw_data(self, data):
        """
        Check if all data has been gathered from socket.
        Parse first line containing message length and check against received message
        :param data: str
        :return: boolean
        """
        length = len(data)
        supposed = data.split('\n')[0][1:-1]
        offset = len(supposed) + 4  # 1 dollar sing, 1 new line character + 1 ending sequence '\r\n'
        if not supposed.isdigit():
            return True
        supposed = int(supposed)

        if length - offset >= supposed:
            self.debug('received full response from redis')
            return True

        self.debug('waiting more data from redis')
        return False

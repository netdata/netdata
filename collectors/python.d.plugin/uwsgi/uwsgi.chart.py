# -*- coding: utf-8 -*-
# Description: uwsgi netdata python.d module
# Author: Robbert Segeren (robbert-ef)
# SPDX-License-Identifier: GPL-3.0-or-later

import json
from copy import deepcopy
from bases.FrameworkServices.SocketService import SocketService

# default module values (can be overridden per job in `config`)
# update_every = 2
priority = 60000
retries = 60

ORDER = [
    'requests',
    'tx',
    'avg_rt',
    'memory_rss',
    'memory_vsz',
    'exceptions',
    'harakiri',
    'respawn',
]

DYNAMIC_CHARTS = [
    'requests',
    'tx',
    'avg_rt',
    'memory_rss',
    'memory_vsz',
]

# NOTE: lines are created dynamically in `check()` method
CHARTS = {
    'requests': {
        'options': [None, 'Requests', 'requests/s', 'requests', 'uwsgi.requests', 'stacked'],
        'lines': [
            ['requests', 'requests', 'incremental']
        ]
    },
    'tx': {
        'options': [None, 'Transmitted data', 'KB/s', 'requests', 'uwsgi.tx', 'stacked'],
        'lines': [
            ['tx', 'tx', 'incremental']
        ]
    },
    'avg_rt': {
        'options': [None, 'Average request time', 'ms', 'requests', 'uwsgi.avg_rt', 'line'],
        'lines': [
            ['avg_rt', 'avg_rt', 'absolute']
        ]
    },
    'memory_rss': {
        'options': [None, 'RSS (Resident Set Size)', 'MB', 'memory', 'uwsgi.memory_rss', 'stacked'],
        'lines': [
            ['memory_rss', 'memory_rss', 'absolute', 1, 1024 * 1024]
        ]
    },
    'memory_vsz': {
        'options': [None, 'VSZ (Virtual Memory Size)', 'MB', 'memory', 'uwsgi.memory_vsz', 'stacked'],
        'lines': [
            ['memory_vsz', 'memory_vsz', 'absolute', 1, 1024 * 1024]
        ]
    },
    'exceptions': {
        'options': [None, 'Exceptions', 'exceptions', 'exceptions', 'uwsgi.exceptions', 'line'],
        'lines': [
            ['exceptions', 'exceptions', 'incremental']
        ]
    },
    'harakiri': {
        'options': [None, 'Harakiris', 'harakiris', 'harakiris', 'uwsgi.harakiris', 'line'],
        'lines': [
            ['harakiri_count', 'harakiris', 'incremental']
        ]
    },
    'respawn': {
        'options': [None, 'Respawns', 'respawns', 'respawns', 'uwsgi.respawns', 'line'],
        'lines': [
            ['respawn_count', 'respawns', 'incremental']
        ]
    },
}


class Service(SocketService):
    def __init__(self, configuration=None, name=None):
        super(Service, self).__init__(configuration=configuration, name=name)
        self.url = self.configuration.get('host', 'localhost')
        self.port = self.configuration.get('port', 1717)
        self.order = ORDER
        self.definitions = deepcopy(CHARTS)

        # Clear dynamic dimensions, these are added during `_get_data()` to allow adding workers at run-time
        for chart in DYNAMIC_CHARTS:
            self.definitions[chart]['lines'] = []

        self.last_result = {}
        self.workers = []

    def read_data(self):
        """
        Read data from socket and parse as JSON.
        :return: (dict) stats
        """
        raw_data = self._get_raw_data()
        if not raw_data:
            return None
        try:
            return json.loads(raw_data)
        except ValueError as err:
            self.error(err)
            return None

    def check(self):
        """
        Parse configuration and check if we can read data.
        :return: boolean
        """
        self._parse_config()
        return bool(self.read_data())

    def add_worker_dimensions(self, key):
        """
        Helper to add dimensions for a worker.
        :param key: (int or str) worker identifier
        :return:
        """
        for chart in DYNAMIC_CHARTS:
            for line in CHARTS[chart]['lines']:
                dimension_id = '{}_{}'.format(line[0], key)
                dimension_name = str(key)

                dimension = [dimension_id, dimension_name] + line[2:]
                self.charts[chart].add_dimension(dimension)

    @staticmethod
    def _check_raw_data(data):
        # The server will close the connection when it's done sending
        # data, so just keep looping until that happens.
        return False

    def _get_data(self):
        """
        Read data from socket
        :return: dict
        """
        stats = self.read_data()
        if not stats:
            return None

        result = {
            'exceptions': 0,
            'harakiri_count': 0,
            'respawn_count': 0,
        }

        for worker in stats['workers']:
            key = worker['pid']

            # Add dimensions for new workers
            if key not in self.workers:
                self.add_worker_dimensions(key)
                self.workers.append(key)

            result['requests_{}'.format(key)] = worker['requests']
            result['tx_{}'.format(key)] = worker['tx']
            result['avg_rt_{}'.format(key)] = worker['avg_rt']

            # avg_rt is not reset by uwsgi, so reset here
            if self.last_result.get('requests_{}'.format(key)) == worker['requests']:
                result['avg_rt_{}'.format(key)] = 0

            result['memory_rss_{}'.format(key)] = worker['rss']
            result['memory_vsz_{}'.format(key)] = worker['vsz']

            result['exceptions'] += worker['exceptions']
            result['harakiri_count'] += worker['harakiri_count']
            result['respawn_count'] += worker['respawn_count']

        self.last_result = result
        return result

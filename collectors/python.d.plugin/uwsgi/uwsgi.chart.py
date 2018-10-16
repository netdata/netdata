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

ORDER = ['requests', 'tx', 'avg_rt', 'memory_rss', 'memory_vsz', 'exceptions', 'harakiri', 'respawn', ]

# NOTE: lines are created dynamically in `check()` method
CHARTS = {
    'requests': {
        'options': [None, 'Requests', 'requests/s', 'requests', 'uwsgi.requests', 'stacked'],
        'lines': [
            ['requests.%s', 'worker %s', 'incremental']
        ]},
    'tx': {
        'options': [None, 'Transmitted data', 'KB/s', 'requests', 'uwsgi.tx', 'stacked'],
        'lines': [
            ['tx.%s', 'worker %s', 'incremental']
        ]},
    'avg_rt': {
        'options': [None, 'Average request time', 'ms', 'requests', 'uwsgi.avg_rt', 'line'],
        'lines': [
            ['avg_rt.%s', 'worker %s', 'absolute']
        ]},
    'memory_rss': {
        'options': [None, 'RSS (Resident Set Size)', 'MB', 'memory', 'uwsgi.memory_rss', 'stacked'],
        'lines': [
            ['memory_rss.%s', 'worker %s', 'absolute', 1, 1024 * 1024]
        ]},
    'memory_vsz': {
        'options': [None, 'VSZ (Virtual Memory Size)', 'MB', 'memory', 'uwsgi.memory_vsz', 'stacked'],
        'lines': [
            ['memory_vsz.%s', 'worker %s', 'absolute', 1, 1024 * 1024]
        ]},
    'exceptions': {
        'options': [None, 'Exceptions', 'exceptions', 'exceptions', 'uwsgi.exceptions', 'line'],
        'lines': [
            ['exceptions.%s', 'worker %s', 'incremental']
        ]},
    'harakiri': {
        'options': [None, 'Harakiris', 'harakiris', 'harakiris', 'uwsgi.harakiris', 'line'],
        'lines': [
            ['harakiri_count.%s', 'worker %s', 'incremental']
        ]},
    'respawn': {
        'options': [None, 'Respawns', 'respawns', 'respawns', 'uwsgi.respawns', 'line'],
        'lines': [
            ['respawn_count.%s', 'worker %s', 'incremental']
        ]},
}


class Service(SocketService):
    def __init__(self, configuration=None, name=None):
        SocketService.__init__(self, configuration=configuration, name=name)
        self.url = self.configuration.get('host', 'localhost')
        self.port = self.configuration.get('port', 1717)
        self.order = deepcopy(ORDER)
        self.definitions = deepcopy(CHARTS)

        self.last_result = {}

    def read_data(self):
        """
        Read data from socket and parse as JSON.
        :return: (dict) stats
        """
        raw_data = self._get_raw_data()
        if not raw_data:
            return None
        return json.loads(raw_data)

    def check(self):
        """
        Parse configuration, check if redis is available, and dynamically create chart lines data
        :return: boolean
        """
        if not SocketService.check(self):
            return False

        stats = self.read_data()
        if not stats:
            return False

        # Generate chart dimensions
        for chart in CHARTS:
            self.definitions[chart]['lines'] = []

            for worker in stats['workers']:
                for line in CHARTS[chart]['lines']:
                    # dimension name and id
                    dimension_name = line[0] % worker['id']
                    dimension_id = line[1] % worker['id']
                    self.definitions[chart]['lines'].append([dimension_name, dimension_id] + line[2:])

        return True

    def _check_raw_data(self, data):
        # Keep reading until disconnected
        return False

    def _get_data(self):
        """
        Read data from socket
        :return: dict
        """
        stats = self.read_data()
        if not stats:
            return None

        result = {}

        for worker in stats['workers']:
            result['requests.%s' % worker['id']] = worker['requests']
            result['tx.%s' % worker['id']] = worker['tx']
            result['avg_rt.%s' % worker['id']] = worker['avg_rt']

            # avg_rt is not reset by uwsgi, so reset here
            if self.last_result.get('requests.%s' % worker['id']) == worker['requests']:
                result['avg_rt.%s' % worker['id']] = 0

            result['memory_rss.%s' % worker['id']] = worker['rss']
            result['memory_vsz.%s' % worker['id']] = worker['vsz']
            result['exceptions.%s' % worker['id']] = worker['exceptions']
            result['harakiri_count.%s' % worker['id']] = worker['harakiri_count']
            result['respawn_count.%s' % worker['id']] = worker['respawn_count']

        self.last_result = result
        return result

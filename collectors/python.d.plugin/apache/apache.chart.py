# -*- coding: utf-8 -*-
# Description: apache netdata python.d module
# Author: Pawel Krupa (paulfantom)
# SPDX-License-Identifier: GPL-3.0-or-later

from bases.FrameworkServices.UrlService import UrlService


ORDER = [
    'requests',
    'connections',
    'conns_async',
    'net',
    'workers',
    'reqpersec',
    'bytespersec',
    'bytesperreq',
]

CHARTS = {
    'bytesperreq': {
        'options': [None, 'Lifetime Avg. Request Size', 'KiB',
                    'statistics', 'apache.bytesperreq', 'area'],
        'lines': [
            ['size_req', 'size', 'absolute', 1, 1024 * 100000]
        ]},
    'workers': {
        'options': [None, 'Workers', 'workers', 'workers', 'apache.workers', 'stacked'],
        'lines': [
            ['idle'],
            ['busy'],
        ]},
    'reqpersec': {
        'options': [None, 'Lifetime Avg. Requests/s', 'requests/s', 'statistics',
                    'apache.reqpersec', 'area'],
        'lines': [
            ['requests_sec', 'requests', 'absolute', 1, 100000]
        ]},
    'bytespersec': {
        'options': [None, 'Lifetime Avg. Bandwidth/s', 'kilobits/s', 'statistics',
                    'apache.bytesperreq', 'area'],
        'lines': [
            ['size_sec', None, 'absolute', 8, 1000 * 100000]
        ]},
    'requests': {
        'options': [None, 'Requests', 'requests/s', 'requests', 'apache.requests', 'line'],
        'lines': [
            ['requests', None, 'incremental']
        ]},
    'net': {
        'options': [None, 'Bandwidth', 'kilobits/s', 'bandwidth', 'apache.net', 'area'],
        'lines': [
            ['sent', None, 'incremental', 8, 1]
        ]},
    'connections': {
        'options': [None, 'Connections', 'connections', 'connections', 'apache.connections', 'line'],
        'lines': [
            ['connections']
        ]},
    'conns_async': {
        'options': [None, 'Async Connections', 'connections', 'connections', 'apache.conns_async', 'stacked'],
        'lines': [
            ['keepalive'],
            ['closing'],
            ['writing']
        ]}
}

ASSIGNMENT = {
    'BytesPerReq': 'size_req',
    'IdleWorkers': 'idle',
    'IdleServers': 'idle_servers',
    'BusyWorkers': 'busy',
    'BusyServers': 'busy_servers',
    'ReqPerSec': 'requests_sec',
    'BytesPerSec': 'size_sec',
    'Total Accesses': 'requests',
    'Total kBytes': 'sent',
    'ConnsTotal': 'connections',
    'ConnsAsyncKeepAlive': 'keepalive',
    'ConnsAsyncClosing': 'closing',
    'ConnsAsyncWriting': 'writing'
}

FLOAT_VALUES = [
    'BytesPerReq',
    'ReqPerSec',
    'BytesPerSec',
]

LIGHTTPD_MARKER = 'idle_servers'


class Service(UrlService):
    def __init__(self, configuration=None, name=None):
        UrlService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.url = self.configuration.get('url', 'http://localhost/server-status?auto')

    def check(self):
        self._manager = self._build_manager()

        data = self._get_data()

        if not data:
            return None

        if LIGHTTPD_MARKER in data:
            self.turn_into_lighttpd()

        return True

    def _get_data(self):
        """
        Format data received from http request
        :return: dict
        """
        raw_data = self._get_raw_data()

        if not raw_data:
            return None

        data = dict()

        for line in raw_data.split('\n'):
            try:
                parse_line(line, data)
            except ValueError:
                continue

        return data or None

    def turn_into_lighttpd(self):
        self.module_name = 'lighttpd'
        for chart in self.definitions:
            if chart == 'workers':
                lines = self.definitions[chart]['lines']
                lines[0] = ['idle_servers', 'idle']
                lines[1] = ['busy_servers', 'busy']
            opts = self.definitions[chart]['options']
            opts[1] = opts[1].replace('apache', 'lighttpd')
            opts[4] = opts[4].replace('apache', 'lighttpd')


def parse_line(line, data):
    parts = line.split(':')

    if len(parts) != 2:
        return

    key, value = parts[0], parts[1]

    if key not in ASSIGNMENT:
        return

    if key in FLOAT_VALUES:
        data[ASSIGNMENT[key]] = int((float(value) * 100000))
    else:
        data[ASSIGNMENT[key]] = int(value)

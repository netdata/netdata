# -*- coding: utf-8 -*-
# Description: traefik netdata python.d module
# Author: Alexandre Menezes (@ale_menezes)
# SPDX-License-Identifier: GPL-3.0-or-later

from json import loads
from collections import defaultdict
from bases.FrameworkServices.UrlService import UrlService

# default module values (can be overridden per job in `config`)
update_every = 1
priority = 60000
retries = 10

# charts order (can be overridden if you want less charts, or different order)
ORDER = [
    'response_statuses',
    'response_codes',
    'detailed_response_codes',
    'requests',
    'total_response_time',
    'average_response_time',
    'average_response_time_per_iteration',
    'uptime'
]

CHARTS = {
    'response_statuses': {
        'options': [None, 'Response statuses', 'requests/s', 'responses', 'traefik.response_statuses', 'stacked'],
        'lines': [
            ['successful_requests', 'success', 'incremental'],
            ['server_errors', 'error', 'incremental'],
            ['redirects', 'redirect', 'incremental'],
            ['bad_requests', 'bad', 'incremental'],
            ['other_requests', 'other', 'incremental']
        ]
    },
    'response_codes': {
        'options': [None, 'Responses by codes', 'requests/s', 'responses', 'traefik.response_codes', 'stacked'],
        'lines': [
            ['2xx', None, 'incremental'],
            ['5xx', None, 'incremental'],
            ['3xx', None, 'incremental'],
            ['4xx', None, 'incremental'],
            ['1xx', None, 'incremental'],
            ['other', None, 'incremental']
        ]
    },
    'detailed_response_codes': {
        'options': [None, 'Detailed response codes', 'requests/s', 'responses', 'traefik.detailed_response_codes',
                    'stacked'],
        'lines': []
    },
    'requests': {
        'options': [None, 'Requests', 'requests/s', 'requests', 'traefik.requests', 'line'],
        'lines': [
            ['total_count', 'requests', 'incremental']
        ]
    },
    'total_response_time': {
        'options': [None, 'Total response time', 'seconds', 'timings', 'traefik.total_response_time', 'line'],
        'lines': [
            ['total_response_time_sec', 'response', 'absolute', 1, 10000]
        ]
    },
    'average_response_time': {
        'options': [None, 'Average response time', 'milliseconds', 'timings', 'traefik.average_response_time', 'line'],
        'lines': [
            ['average_response_time_sec', 'response', 'absolute', 1, 1000]
        ]
    },
    'average_response_time_per_iteration': {
        'options': [None, 'Average response time per iteration', 'milliseconds', 'timings',
                    'traefik.average_response_time_per_iteration', 'line'],
        'lines': [
            ['average_response_time_per_iteration_sec', 'response', 'incremental', 1, 10000]
        ]
    },
    'uptime': {
        'options': [None, 'Uptime', 'seconds', 'uptime', 'traefik.uptime', 'line'],
        'lines': [
            ['uptime_sec', 'uptime', 'absolute']
        ]
    }
}

HEALTH_STATS = [
    'uptime_sec',
    'average_response_time_sec',
    'total_response_time_sec',
    'total_count',
    'total_status_code_count'
]


class Service(UrlService):
    def __init__(self, configuration=None, name=None):
        UrlService.__init__(self, configuration=configuration, name=name)
        self.url = self.configuration.get('url', 'http://localhost:8080/health')
        self.order = ORDER
        self.definitions = CHARTS
        self.data = {
            'successful_requests': 0, 'redirects': 0, 'bad_requests': 0,
            'server_errors': 0, 'other_requests': 0, '1xx': 0, '2xx': 0,
            '3xx': 0, '4xx': 0, '5xx': 0, 'other': 0,
            'average_response_time_per_iteration_sec': 0
        }
        self.last_total_response_time = 0
        self.last_total_count = 0

    def _get_data(self):
        data = self._get_raw_data()

        if not data:
            return None

        data = loads(data)

        self.get_data_per_code_status(raw_data=data)

        self.get_data_per_code_family(raw_data=data)

        self.get_data_per_code(raw_data=data)

        self.data.update(fetch_data_(raw_data=data, metrics=HEALTH_STATS))

        self.data['average_response_time_sec'] *= 1000000
        self.data['total_response_time_sec'] *= 10000
        if data['total_count'] != self.last_total_count:
            self.data['average_response_time_per_iteration_sec'] = \
                (data['total_response_time_sec'] - self.last_total_response_time) * \
                1000000 / (data['total_count'] - self.last_total_count)
        else:
            self.data['average_response_time_per_iteration_sec'] = 0
        self.last_total_response_time = data['total_response_time_sec']
        self.last_total_count = data['total_count']

        return self.data or None

    def get_data_per_code_status(self, raw_data):
        data = defaultdict(int)
        for code, value in raw_data['total_status_code_count'].items():
            code_prefix = code[0]
            if code_prefix == '1' or code_prefix == '2' or code == '304':
                data['successful_requests'] += value
            elif code_prefix == '3':
                data['redirects'] += value
            elif code_prefix == '4':
                data['bad_requests'] += value
            elif code_prefix == '5':
                data['server_errors'] += value
            else:
                data['other_requests'] += value
        self.data.update(data)

    def get_data_per_code_family(self, raw_data):
        data = defaultdict(int)
        for code, value in raw_data['total_status_code_count'].items():
            code_prefix = code[0]
            if code_prefix == '1':
                data['1xx'] += value
            elif code_prefix == '2':
                data['2xx'] += value
            elif code_prefix == '3':
                data['3xx'] += value
            elif code_prefix == '4':
                data['4xx'] += value
            elif code_prefix == '5':
                data['5xx'] += value
            else:
                data['other'] += value
        self.data.update(data)

    def get_data_per_code(self, raw_data):
        for code, value in raw_data['total_status_code_count'].items():
            if self.charts:
                if code not in self.data:
                    self.charts['detailed_response_codes'].add_dimension([code, code, 'incremental'])
                self.data[code] = value


def fetch_data_(raw_data, metrics):
    data = dict()

    for metric in metrics:
        value = raw_data
        metrics_list = metric.split('.')
        try:
            for m in metrics_list:
                value = value[m]
        except KeyError:
            continue
        data['_'.join(metrics_list)] = value

    return data

# -*- coding: utf-8 -*-
# Description: traefik netdata python.d module
# Author: Alexandre Menezes (@ale_menezes)

from json import loads
from bases.FrameworkServices.UrlService import UrlService

# default module values (can be overridden per job in `config`)
update_every = 1
priority = 60000
retries = 10

HEALTH_STATS = [
    'average_response_time_sec',
    'total_response_time_sec',
    'total_count',
    'total_status_code_count',
    'uptime_sec'
]

# charts order (can be overridden if you want less charts, or different order)
ORDER = [
    'uptime', 'average_response_time', 'total_response_time', 'requests', 'response_statuses', 'responses_summary'
    ]

CHARTS = {
    'uptime': {
        'options': [None, 'Uptime', 'seconds', 'uptime', 'traefik.uptime', 'line'],
        'lines': [
            ['uptime_sec', None, 'absolute']
        ]},
    'average_response_time': {
        'options': [None, 'Average response time', 'milliseconds', 'average response time', 'traefik.average_response_time', 'line'],
        'lines': [
            ['average_response_time_sec', None, 'incremental', 1000000, 1000]
        ]},
    'total_response_time': {
        'options': [None, 'Total Response Time', 'seconds', 'total response time', 'traefik.total_response_time', 'line'],
        'lines': [
            ['total_response_time_sec', None, 'absolute', 1, 1]
        ]},
    'requests': {
        'options': [None, 'Requests', 'requests/s', 'requests', 'traefik.requests', 'line'],
        'lines': [
            ['total_count', 'requests', 'incremental']
        ]},
    'response_statuses': {
        'options': [None, 'Response Statuses', 'requests/s', 'responses statuses', 'traefik.response_statuses', 'stacked'],
        'lines': [
            ['successful_requests', 'success', 'incremental'],
            ['redirects', 'redirect', 'incremental'],
            ['server_errors', 'error', 'incremental'],
            ['bad_requests', 'bad', 'incremental'],
            ['other_requests', 'other', 'incremental']
        ]},
    'responses_summary': {
        'options': [None, 'Responses Summary', 'requests/s', 'responses summary', 'traefik.response_summary', 'stacked'],
        'lines': [
            ['1xx', None, 'incremental'],
            ['2xx', None, 'incremental'],
            ['3xx', None, 'incremental'],
            ['4xx', None, 'incremental'],
            ['5xx', None, 'incremental'],
            ['unmatched', None, 'incremental']
        ]}
    }


class Service(UrlService):
    def __init__(self, configuration=None, name=None):
        UrlService.__init__(self, configuration=configuration, name=name)
        self.url = self.configuration.get('url', 'http://localhost:8080/health')
        self.order = ORDER
        self.definitions = CHARTS
        self.data = {'1xx': 0, '2xx': 0, '3xx': 0, '4xx': 0,
                '5xx': 0, 'unmatched': 0, 'successful_requests': 0,
                'redirects': 0, 'bad_requests': 0, 'server_errors': 0,
                'other_requests': 0}

    def _get_data(self):
        data = self._get_raw_data()

        if not data:
            return None

        data = loads(data)
        self.get_data_per_code(raw_data=data)
        to_netdata = fetch_data_(raw_data=data, metrics=HEALTH_STATS)
        to_netdata.update(self.data)
        self.data = {'1xx': 0, '2xx': 0, '3xx': 0, '4xx': 0,
                '5xx': 0, 'unmatched': 0, 'successful_requests': 0,
                'redirects': 0, 'bad_requests': 0, 'server_errors': 0,
                'other_requests': 0}
        return to_netdata or None

    def get_data_per_code(self, raw_data):
        for code in raw_data['total_status_code_count']:
            code_prefix = list(code)[0]
            if code_prefix == '1':
                self.data['1xx'] = raw_data['total_status_code_count'][code]
                self.data['successful_requests'] += raw_data['total_status_code_count'][code]
            elif code_prefix == '2':
                self.data['2xx'] = raw_data['total_status_code_count'][code]
                self.data['successful_requests'] += raw_data['total_status_code_count'][code]
            elif code_prefix == '304':
                self.data['successful_requests'] += raw_data['total_status_code_count'][code]
            elif code_prefix == '3':
                self.data['3xx'] = raw_data['total_status_code_count'][code]
                self.data['redirects'] += raw_data['total_status_code_count'][code]
            elif code_prefix == '4':
                self.data['4xx'] = raw_data['total_status_code_count'][code]
                self.data['bad_requests'] += raw_data['total_status_code_count'][code]
            elif code_prefix == '5':
                self.data['5xx'] = raw_data['total_status_code_count'][code]
                self.data['server_errors'] += raw_data['total_status_code_count'][code]
            else:
                self.data['unmatched'] = raw_data['total_status_code_count'][code]
                self.data['other_requests'] += raw_data['total_status_code_count'][code]

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

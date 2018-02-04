# -*- coding: utf-8 -*-
# Description: traefik netdata python.d module
# Author: Alexandre Menezes (@ale_menezes)

from json import loads

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
            ['redirects', 'redirect', 'incremental'],
            ['server_errors', 'error', 'incremental'],
            ['bad_requests', 'bad', 'incremental'],
            ['other_requests', 'other', 'incremental']
        ]},
    'response_codes': {
        'options': [None, 'Responses by codes', 'requests/s', 'responses', 'traefik.response_codes', 'stacked'],
        'lines': [
            ['1xx', None, 'incremental'],
            ['2xx', None, 'incremental'],
            ['3xx', None, 'incremental'],
            ['4xx', None, 'incremental'],
            ['5xx', None, 'incremental'],
            ['unmatched', None, 'incremental']
        ]},
    'detailed_response_codes': {
        'options': [None, 'Detailed response codes', 'requests/s', 'responses', 'traefik.detailed_response_codes', 'stacked'],
        'lines': [
            ['None', None, 'incremental']
        ]},
    'requests': {
        'options': [None, 'Requests', 'requests/s', 'requests', 'traefik.requests', 'line'],
        'lines': [
            ['total_count', 'requests', 'incremental']
        ]},
    'total_response_time': {
        'options': [None, 'Total response time', 'seconds', 'timings', 'traefik.total_response_time', 'line'],
        'lines': [
            ['total_response_time_sec', None, 'absolute', 1, 1]
        ]},
    'average_response_time': {
        'options': [None, 'Average response time', 'milliseconds', 'timings', 'traefik.average_response_time', 'line'],
        'lines': [
            ['average_response_time_sec', None, 'incremental', 1000000, 1000]
        ]},
    'average_response_time_per_iteration': {
        'options': [None, 'Average response time per iteration', 'milliseconds', 'timings', 'traefik.average_response_time_per_iteration', 'line'],
        'lines': [
            ['average_response_time_per_iteration_sec', None, 'incremental', 1, 1000]
        ]},
    'uptime': {
        'options': [None, 'Uptime', 'seconds', 'uptime', 'traefik.uptime', 'line'],
        'lines': [
            ['uptime_sec', None, 'absolute']
        ]}
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
            'None': 0,
            'average_response_time_per_iteration_sec': 0
        }

    def check(self):
        if not (self.url):
            self.error('url is not defined in the module configuration file')
            return False

        self._manager = self._build_manager()
        data = self._get_raw_data(self.url)
        data = loads(data)

        for code in data['total_status_code_count']:
            self.definitions['detailed_response_codes']['lines'].append(
                    [code, code, 'incremental']
            )
            if code not in self.data:
                self.data[code] = data['total_status_code_count'][code]
        return UrlService.check(self)

    def _get_data(self):
        data = self._get_raw_data()

        if not data:
            return None

        data = loads(data)

        self.get_data_per_code_status(raw_data=data)

        self.get_data_per_code_family(raw_data=data)

        self.get_data_per_code(raw_data=data)

        to_netdata = fetch_data_(raw_data=data, metrics=HEALTH_STATS)
        to_netdata.update(self.data)

        self.data['average_response_time_per_iteration_sec'] = (data['total_response_time_sec'] * 1000000) / data['total_count']
        to_netdata.update(self.data)

        return to_netdata or None

    def get_data_per_code_status(self, raw_data):
        code_status = {'successful_requests': 0, 'redirects': 0,
                       'bad_requests': 0, 'server_errors': 0,
                       'other_requests': 0}
        for code in raw_data['total_status_code_count']:
            code_prefix = code[0]
            if code_prefix == '1' or code_prefix == '2' or code == '304':
                code_status['successful_requests'] += raw_data['total_status_code_count'][code]
            elif code_prefix == '3':
                code_status['redirects'] += raw_data['total_status_code_count'][code]
            elif code_prefix == '4':
                code_status['bad_requests'] += raw_data['total_status_code_count'][code]
            elif code_prefix == '5':
                code_status['server_errors'] += raw_data['total_status_code_count'][code]
            else:
                code_status['other_requests'] += raw_data['total_status_code_count'][code]
        self.data.update(code_status)

    def get_data_per_code_family(self, raw_data):
        code_family = {'1xx': 0, '2xx': 0, '3xx': 0, '4xx': 0,
                '5xx': 0, 'unmatched': 0}
        for code in raw_data['total_status_code_count']:
            code_prefix = code[0]
            if code_prefix == '1':
                code_family['1xx'] += raw_data['total_status_code_count'][code]
            elif code_prefix == '2':
                code_family['2xx'] += raw_data['total_status_code_count'][code]
            elif code_prefix == '3':
                code_family['3xx'] += raw_data['total_status_code_count'][code]
            elif code_prefix == '4':
                code_family['4xx'] += raw_data['total_status_code_count'][code]
            elif code_prefix == '5':
                code_family['5xx'] += raw_data['total_status_code_count'][code]
            else:
                code_family['unmatched'] += raw_data['total_status_code_count'][code]
        self.data.update(code_family)

    def get_data_per_code(self, raw_data):
        for code in raw_data['total_status_code_count']:
            self.add_chart_dimension(code)
            self.data[code] = raw_data['total_status_code_count'][code]

    def add_chart_dimension(self, code):
        if code not in self.data:
            self.charts['detailed_response_codes'].add_dimension([code, code, 'incremental'])
            self.data[code] = 0

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
